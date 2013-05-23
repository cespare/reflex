package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/howeyc/fsnotify"
	"github.com/kballard/go-shellquote"
)

const (
	defaultSubSymbol = "{}"
)

var (
	matchAll = regexp.MustCompile(".*")

	flagConf     string
	verbose      bool
	globalFlags  = flag.NewFlagSet("", flag.ContinueOnError)
	globalConfig = &Config{}
)

type Config struct {
	regex string
	glob  string
	start bool
}

func init() {
	globalFlags.StringVar(&flagConf, "c", "", "A configuration file that describes how to run reflex.")
	globalFlags.BoolVar(&verbose, "v", false, "Verbose mode: print out more information about what reflex is doing.")
	registerFlags(globalFlags, globalConfig)
}

func registerFlags(f *flag.FlagSet, config *Config) {
	f.StringVar(&config.regex, "r", "", "A regular expression to match filenames.")
	f.StringVar(&config.glob, "g", "", "A shell glob expression to match filenames.")
	f.BoolVar(&config.start, "s", false,
		"Indicates that the command is a long-running process to be restarted on matching changes.")
}

func parseMatchers(rs, gs string) (regex *regexp.Regexp, glob string, err error) {
	if rs == "" && gs == "" {
		return matchAll, "", nil
	}
	if rs == "" {
		return nil, gs, nil
	}
	if gs == "" {
		regex, err := regexp.Compile(rs)
		if err != nil {
			return nil, "", err
		}
		return regex, "", nil
	}
	return nil, "", errors.New("Both regex and glob specified.")
}

func Fatalln(args ...interface{}) {
	fmt.Println(args...)
	os.Exit(1)
}

func walker(watcher *fsnotify.Watcher) filepath.WalkFunc {
	return func(path string, f os.FileInfo, err error) error {
		if err != nil || !f.IsDir() {
			// TODO: Is there some other thing we should be doing to handle errors? When watching large
			// directories that have lots of programs modifying them (esp. if they're making tempfiles along the
			// way), we often get errors.
			return nil
		}
		fmt.Println("Adding watch for path", path)
		if err := watcher.Watch(path); err != nil {
			// TODO: handle this somehow?
			fmt.Printf("Error while watching new path %s: %s\n", path, err)
		}
		return nil
	}
}

func broadcast(in <-chan string, outs []chan<- string) {
	for e := range in {
		for _, out := range outs {
			out <- e
		}
	}
}

func watch(root string, watcher *fsnotify.Watcher, names chan<- string, done chan<- error) {
	if err := watcher.Watch(root); err != nil {
		Fatalln(err)
	}

	for {
		select {
		case e := <-watcher.Event:
			path := strings.TrimPrefix(e.Name, "./")
			names <- path
			if e.IsCreate() {
				if err := filepath.Walk(path, walker(watcher)); err != nil {
					// TODO: handle this somehow?
					fmt.Printf("Error while walking path %s: %s\n", path, err)
				}
			}
			if e.IsDelete() {
				watcher.RemoveWatch(path)
			}
		case err := <-watcher.Error:
			done <- err
			return
		}
	}
}

// runCommand runs the given Command. Blocks until the command exits. All output is passed line-by-line to the
// stderr/stdout channels.
func runCommand(cmd *exec.Cmd, stdout chan<- string, stderr chan<- string) error {
	cmdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stdoutErr := make(chan error)
	go func() {
		scanner := bufio.NewScanner(cmdout)
		for scanner.Scan() {
			stdout <- scanner.Text()
		}
		stdoutErr <- scanner.Err()
	}()

	stderrErr := make(chan error)
	go func() {
		scanner := bufio.NewScanner(cmderr)
		for scanner.Scan() {
			stderr <- scanner.Text()
		}
		stderrErr <- scanner.Err()
	}()

	cmdErr := make(chan error)
	go func() {
		cmdErr <- cmd.Wait()
	}()

	for {
		select {
		case err := <-stdoutErr:
			if err != nil {
				return err
			}
			stdoutErr = nil
		case err := <-stderrErr:
			if err != nil {
				return err
			}
			stderrErr = nil
		case err := <-cmdErr:
			if err != nil {
				stderr <- fmt.Sprintf("(error exit: %s)\n", err)
			}
			cmdErr = nil
		}
		if stdoutErr == nil && stderrErr == nil && cmdErr == nil {
			break
		}
	}

	return nil
}

// filterMatchingRegex passes on messages matching regex.
func filterMatchingRegex(in <-chan string, out chan<- string, regex *regexp.Regexp) {
	for name := range in {
		if regex.MatchString(name) {
			out <- name
		}
	}
}

// filterMatchingGlob passes on messages matching glob.
func filterMatchingGlob(in <-chan string, out chan<- string, glob string) {
	for name := range in {
		matches, err := filepath.Match(glob, name)
		// TODO: It would be good to notify the user on an error here.
		if err == nil && matches {
			out <- name
		}
	}
}

// batchRun receives realtime file notification events and batches them up. It's a bit tricky, but here's what
// it accomplishes:
// * When we initially get a message, wait a bit and batch messages before trying to send anything. This is
//	 because the file events come in quick bursts.
// * Once it's time to send, don't do it until the out channel is unblocked. In the meantime, keep batching.
//   When we've sent off all the batched messages, go back to the beginning.
func batchRun(in <-chan string, out chan<- string, backlog Backlog) {
	for name := range in {
		backlog.Add(name)
		timer := time.NewTimer(200 * time.Millisecond)
	outer:
		for {
			select {
			case name := <-in:
				backlog.Add(name)
			case <-timer.C:
				for {
					select {
					case name := <-in:
						backlog.Add(name)
					case out <- backlog.Next():
						if backlog.RemoveOne() {
							break outer
						}
					}
				}
			}
		}
	}
}

// runEach runs the command on each name that comes through the names channel. Each {} is replaced by the name
// of the file. The stderr and stdout of the command are passed line-by-line to the stderr and stdout chans.
func runEach(names <-chan string, stdout chan<- string, stderr chan<- string, command []string) {
	for name := range names {
		replacer := strings.NewReplacer(defaultSubSymbol, name)
		args := make([]string, len(command))
		for i, c := range command {
			args[i] = replacer.Replace(c)
		}
		cmd := exec.Command(args[0], args[1:]...)

		runCommand(cmd, stdout, stderr)
	}
}

func printOutput(out <-chan string, writer io.Writer) {
	for line := range out {
		fmt.Fprintln(writer, line)
	}
}

// This ties together a single reflex 'instance' so that multiple watches/commands can be handled together
// easily.
type Reflex struct {
	start    bool
	backlog  Backlog
	regex    *regexp.Regexp
	glob     string
	useRegex bool
	command  []string

	done       chan error
	rawChanges chan string
	filtered   chan string
	batched    chan string
}

func NewReflex(regexString, globString string, command []string, start bool, subSymbol string) (*Reflex, error) {
	regex, glob, err := parseMatchers(regexString, globString)
	if err != nil {
		Fatalln("Error parsing glob/regex.\n" + err.Error())
	}
	if len(command) == 0 {
		return nil, errors.New("Must give command to execute.")
	}

	if subSymbol == "" {
		subSymbol = defaultSubSymbol
	}

	substitution := false
	for _, part := range command {
		if strings.Contains(part, subSymbol) {
			substitution = true
			break
		}
	}

	var backlog Backlog
	if substitution {
		backlog = &UniqueFilesBacklog{true, "", make(map[string]struct{})}
	} else {
		backlog = new(UnifiedBacklog)
	}

	reflex := &Reflex{
		start:    start,
		backlog:  backlog,
		regex:    regex,
		glob:     glob,
		useRegex: regex != nil,
		command:  command,

		rawChanges: make(chan string),
		filtered:   make(chan string),
		batched:    make(chan string),
	}

	return reflex, nil
}

func (r *Reflex) PrintInfo(source string) {
	fmt.Println("Reflex from", source)
	if r.regex == matchAll {
		fmt.Println("| No regex (-r) or glob (-g) given, so matching all file changes.")
	} else if r.useRegex {
		fmt.Println("| Regex:", r.regex)
	} else {
		fmt.Println("| Glob:", r.glob)
	}
	fmt.Println("| Command:", r.command)
	fmt.Println("+---------")
}

func main() {
	if err := globalFlags.Parse(os.Args[1:]); err != nil {
		Fatalln(err)
	}

	reflexes := []*Reflex{}

	if flagConf == "" {
		reflex, err := NewReflex(globalConfig.regex, globalConfig.glob, globalFlags.Args(), false, "")
		if err != nil {
			Fatalln(err)
		}
		if verbose {
			reflex.PrintInfo("commandline")
		}
		reflexes = append(reflexes, reflex)
	} else {
		if flag.NFlag() > 1 {
			Fatalln("Cannot set other flags along with -c.")
		}
		configFile, err := os.Open(flagConf)
		if err != nil {
			Fatalln(err)
		}
		scanner := bufio.NewScanner(configFile)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			errorMsg := fmt.Sprintf("Error on line: %d", lineNo)
			config := &Config{}
			flags := flag.NewFlagSet("", flag.ContinueOnError)
			registerFlags(flags, config)
			parts, err := shellquote.Split(scanner.Text())
			if err != nil {
				Fatalln(errorMsg, err)
			}
			// Skip empty lines and comments (lines starting with #).
			if len(parts) == 0 || strings.HasPrefix(parts[0], "#") {
				continue
			}
			if err := flags.Parse(parts); err != nil {
				Fatalln(errorMsg, err)
			}
			reflex, err := NewReflex(config.regex, config.glob, flags.Args(), false, "")
			if err != nil {
				Fatalln(errorMsg, err)
			}
			if verbose {
				reflex.PrintInfo(fmt.Sprintf("%s, line %d", flagConf, lineNo))
			}
			reflexes = append(reflexes, reflex)
		}
		if err := scanner.Err(); err != nil {
			Fatalln(err)
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		Fatalln(err)
	}
	defer watcher.Close()

	rawChanges := make(chan string)
	allRawChanges := make([]chan<- string, len(reflexes))
	done := make(chan error)
	for i, reflex := range reflexes {
		allRawChanges[i] = reflex.rawChanges
	}
	go watch(".", watcher, rawChanges, done)
	go broadcast(rawChanges, allRawChanges)

	stdout := make(chan string, 100)
	stderr := make(chan string, 100)
	go printOutput(stdout, os.Stdout)
	go printOutput(stderr, os.Stderr)

	for _, reflex := range reflexes {
		if reflex.useRegex {
			go filterMatchingRegex(reflex.rawChanges, reflex.filtered, reflex.regex)
		} else {
			go filterMatchingGlob(reflex.rawChanges, reflex.filtered, reflex.glob)
		}
		go batchRun(reflex.filtered, reflex.batched, reflex.backlog)
		go runEach(reflex.batched, stdout, stderr, reflex.command)
	}

	Fatalln(<-done)
}
