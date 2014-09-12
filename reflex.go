package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	flag "github.com/cespare/pflag"
	"github.com/howeyc/fsnotify"
)

const defaultSubSymbol = "{}"

var (
	reflexes []*Reflex
	matchAll = regexp.MustCompile(".*")

	flagConf       string
	flagSequential bool
	flagDecoration string
	decoration     Decoration
	verbose        bool
	globalFlags    = flag.NewFlagSet("", flag.ContinueOnError)
	globalConfig   = &Config{}

	reflexID = 0
	stdout   = make(chan OutMsg, 1)

	cleanupMut = &sync.Mutex{}
)

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s [OPTIONS] [COMMAND]

COMMAND is any command you'd like to run. Any instance of {} will be replaced
with the filename of the changed file. (The symbol may be changed with the
--substitute flag.)

OPTIONS are given below:
`, os.Args[0])

	globalFlags.PrintDefaults()

	fmt.Fprintln(os.Stderr, `
Examples:

    # Print each .txt file if it changes
    $ reflex -r '\.txt$' echo {}

    # Run 'make' if any of the .c files in this directory change:
    $ reflex -g '*.c' make

    # Build and run a server; rebuild and restart when .java files change:
    $ reflex -r '\.java$' -s -- sh -c 'make && java bin/Server'
`)
}

func init() {
	globalFlags.Usage = usage
	globalFlags.StringVarP(&flagConf, "config", "c", "", `
            A configuration file that describes how to run reflex
            (or '-' to read the configuration from stdin).`)
	globalFlags.BoolVarP(&verbose, "verbose", "v", false, `
            Verbose mode: print out more information about what reflex is doing.`)
	globalFlags.BoolVarP(&flagSequential, "sequential", "e", false, `
            Don't run multiple commands at the same time.`)
	globalFlags.StringVarP(&flagDecoration, "decoration", "d", "plain", `
            How to decorate command output. Choices: none, plain, fancy.`)
	globalConfig.registerFlags(globalFlags)
}

func anyNonGlobalsRegistered() bool {
	any := false
	walkFn := func(f *flag.Flag) {
		if !(f.Name == "config" || f.Name == "verbose" || f.Name == "sequential" || f.Name == "decoration") {
			any = any || true
		}
	}
	globalFlags.Visit(walkFn)
	return any
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

// This ties together a single reflex 'instance' so that multiple watches/commands can be handled together
// easily.
type Reflex struct {
	id           int
	source       string // Describes what config/line defines this Reflex
	startService bool
	backlog      Backlog
	regex        *regexp.Regexp
	glob         string
	useRegex     bool
	onlyFiles    bool
	onlyDirs     bool
	command      []string
	subSymbol    string

	done       chan struct{}
	rawChanges chan string
	filtered   chan string
	batched    chan string

	// Used for services (startService = true)
	cmd    *exec.Cmd
	tty    *os.File
	mut    *sync.Mutex // protects killed
	killed bool
}

// This function is not threadsafe.
func NewReflex(c *Config) (*Reflex, error) {
	regex, glob, err := parseMatchers(c.regex, c.glob)
	if err != nil {
		Fatalln("Error parsing glob/regex.\n" + err.Error())
	}
	if len(c.command) == 0 {
		return nil, errors.New("Must give command to execute.")
	}

	if c.subSymbol == "" {
		return nil, errors.New("Substitution symbol must be non-empty.")
	}

	substitution := false
	for _, part := range c.command {
		if strings.Contains(part, c.subSymbol) {
			substitution = true
			break
		}
	}

	var backlog Backlog
	if substitution {
		if c.startService {
			return nil, errors.New("Using --start-service does not work with a command that has a substitution symbol.")
		}
		backlog = NewUniqueFilesBacklog()
	} else {
		backlog = NewUnifiedBacklog()
	}

	if c.onlyFiles && c.onlyDirs {
		return nil, errors.New("Cannot specify both --only-files and --only-dirs.")
	}

	reflex := &Reflex{
		id:           reflexID,
		source:       c.source,
		startService: c.startService,
		backlog:      backlog,
		regex:        regex,
		glob:         glob,
		useRegex:     regex != nil,
		onlyFiles:    c.onlyFiles,
		onlyDirs:     c.onlyDirs,
		command:      c.command,
		subSymbol:    c.subSymbol,

		rawChanges: make(chan string),
		filtered:   make(chan string),
		batched:    make(chan string),

		mut: &sync.Mutex{},
	}
	reflexID++

	return reflex, nil
}

func (r *Reflex) PrintInfo() {
	fmt.Println("Reflex from", r.source)
	fmt.Println("| ID:", r.id)
	if r.regex == matchAll {
		fmt.Println("| No regex (-r) or glob (-g) given, so matching all file changes.")
	} else if r.useRegex {
		fmt.Println("| Regex:", r.regex)
	} else {
		fmt.Println("| Glob:", r.glob)
	}
	if r.onlyFiles {
		fmt.Println("| Only matching files.")
	} else if r.onlyDirs {
		fmt.Println("| Only matching directories.")
	}
	if !r.startService {
		fmt.Println("| Substitution symbol", r.subSymbol)
	}
	replacer := strings.NewReplacer(r.subSymbol, "<filename>")
	command := make([]string, len(r.command))
	for i, part := range r.command {
		command[i] = replacer.Replace(part)
	}
	fmt.Println("| Command:", command)
	fmt.Println("+---------")
}

func printGlobals() {
	fmt.Println("Globals set at commandline")
	walkFn := func(f *flag.Flag) {
		fmt.Printf("| --%s (-%s) '%s' (default: '%s')\n", f.Name, f.Shorthand, f.Value, f.DefValue)
	}
	globalFlags.Visit(walkFn)
	fmt.Println("+---------")
}

func cleanup(reason string) {
	cleanupMut.Lock()
	defer cleanupMut.Unlock()
	fmt.Println(reason)
	wg := &sync.WaitGroup{}
	for _, reflex := range reflexes {
		if reflex.done != nil {
			wg.Add(1)
			go func(reflex *Reflex) {
				terminate(reflex)
				wg.Done()
			}(reflex)
		}
	}
	wg.Wait()
	// Give just a little time to finish printing output.
	time.Sleep(10 * time.Millisecond)
	os.Exit(0)
}

func main() {
	if err := globalFlags.Parse(os.Args[1:]); err != nil {
		Fatalln(err)
	}
	globalConfig.command = globalFlags.Args()
	globalConfig.source = "[commandline]"
	if verbose {
		printGlobals()
	}
	switch strings.ToLower(flagDecoration) {
	case "none":
		decoration = DecorationNone
	case "plain":
		decoration = DecorationPlain
	case "fancy":
		decoration = DecorationFancy
	default:
		Fatalln(fmt.Sprintf("Invalid decoration %s. Choices: none, plain, fancy.", flagDecoration))
	}

	var configs []*Config
	if flagConf == "" {
		if flagSequential {
			Fatalln("Cannot set --sequential without --config (because you cannot specify multiple commands).")
		}
		configs = []*Config{globalConfig}
	} else {
		if anyNonGlobalsRegistered() {
			Fatalln("Cannot set other flags along with --config other than --sequential, --verbose, and --decoration.")
		}
		var err error
		configs, err = ReadConfigs(flagConf)
		if err != nil {
			Fatalln("Could not parse configs: ", err)
		}
	}

	for _, config := range configs {
		reflex, err := NewReflex(config)
		if err != nil {
			Fatalln("Could not make reflex for config:", err)
		}
		if verbose {
			reflex.PrintInfo()
		}
		reflexes = append(reflexes, reflex)
	}

	// Catch ctrl-c and make sure to kill off children.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	signal.Notify(signals, os.Signal(syscall.SIGTERM))
	go func() {
		s := <-signals
		reason := fmt.Sprintf("Interrupted (%s). Cleaning up children...", s)
		cleanup(reason)
	}()
	defer cleanup("Cleaning up.")

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

	go printOutput(stdout, os.Stdout)

	for _, reflex := range reflexes {
		go filterMatching(reflex.rawChanges, reflex.filtered, reflex)
		go batch(reflex.filtered, reflex.batched, reflex)
		go runEach(reflex.batched, reflex)
		if reflex.startService {
			// Easy hack to kick off the initial start.
			infoPrintln(reflex.id, "Starting service")
			runCommand(reflex, "", stdout)
		}
	}

	Fatalln(<-done)
}
