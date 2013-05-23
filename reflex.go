package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/kballard/go-shellquote"
	"github.com/howeyc/fsnotify"
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
	regex     string
	glob      string
	subSymbol string
	start     bool
}

func init() {
	globalFlags.StringVar(&flagConf, "c", "", "A configuration file that describes how to run reflex.")
	globalFlags.BoolVar(&verbose, "v", false, "Verbose mode: print out more information about what reflex is doing.")
	registerFlags(globalFlags, globalConfig)
}

func registerFlags(f *flag.FlagSet, config *Config) {
	f.StringVar(&config.regex, "r", "", "A regular expression to match filenames.")
	f.StringVar(&config.glob, "g", "", "A shell glob expression to match filenames.")
	f.StringVar(&config.subSymbol, "sub", defaultSubSymbol,
		"Indicates that the command is a long-running process to be restarted on matching changes.")
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

// This ties together a single reflex 'instance' so that multiple watches/commands can be handled together
// easily.
type Reflex struct {
	start     bool
	backlog   Backlog
	regex     *regexp.Regexp
	glob      string
	useRegex  bool
	command   []string
	subSymbol string

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
		return nil, errors.New("Substitution symbol must be non-empty.")
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
		start:     start,
		backlog:   backlog,
		regex:     regex,
		glob:      glob,
		useRegex:  regex != nil,
		command:   command,
		subSymbol: subSymbol,

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
	replacer := strings.NewReplacer(r.subSymbol, "<filaname>")
	command := make([]string, len(r.command))
	for i, part := range r.command {
		command[i] = replacer.Replace(part)
	}
	fmt.Println("| Command:", command)
	fmt.Println("+---------")
}

func main() {
	if err := globalFlags.Parse(os.Args[1:]); err != nil {
		Fatalln(err)
	}

	reflexes := []*Reflex{}

	if flagConf == "" {
		reflex, err := NewReflex(globalConfig.regex, globalConfig.glob, globalFlags.Args(), false, globalConfig.subSymbol)
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
			reflex, err := NewReflex(config.regex, config.glob, flags.Args(), false, config.subSymbol)
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
		go runEach(reflex.batched, stdout, stderr, reflex.command, reflex.subSymbol)
	}

	Fatalln(<-done)
}
