package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/howeyc/fsnotify"
	"github.com/kballard/go-shellquote"
	flag "github.com/ogier/pflag"
)

const (
	defaultSubSymbol = "{}"
)

type Decoration int

const (
	DecorationNone = iota
	DecorationPlain
	DecorationFancy
)

var (
	matchAll = regexp.MustCompile(".*")

	flagConf       string
	flagSequential bool
	flagDecoration string
	decoration     Decoration
	verbose        bool
	globalFlags    = flag.NewFlagSet("", flag.ContinueOnError)
	globalConfig   = &Config{}

	reflexID = 0
)

type Config struct {
	regex     string
	glob      string
	subSymbol string
	start     bool
	onlyFiles bool
	onlyDirs  bool
}

func init() {
	globalFlags.StringVarP(&flagConf, "config", "c", "", "A configuration file that describes how to run reflex.")
	globalFlags.BoolVarP(&verbose,
		"verbose", "v", false, "Verbose mode: print out more information about what reflex is doing.")
	globalFlags.BoolVarP(&flagSequential,
		"sequential", "e", false, "Don't run multiple commands at the same time.")
	globalFlags.StringVarP(&flagDecoration,
		"decoration", "d", "plain", "How to decorate command stderr/stdout. Choices: none, plain, fancy.")
	registerFlags(globalFlags, globalConfig)
}

func registerFlags(f *flag.FlagSet, config *Config) {
	f.StringVarP(&config.regex, "regex", "r", "", "A regular expression to match filenames.")
	f.StringVarP(&config.glob, "glob", "g", "", "A shell glob expression to match filenames.")
	f.StringVarP(&config.subSymbol, "substitute", "u", defaultSubSymbol,
		"Indicates that the command is a long-running process to be restarted on matching changes.")
	f.BoolVarP(&config.start, "start", "s", false,
		"The substitution symbol that is replaced with the filename in a command.")
	f.BoolVar(&config.onlyFiles, "only-files", false, "Only match files (not directories).")
	f.BoolVar(&config.onlyDirs, "only-dirs", false, "Only match directories (not files).")
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

func Fatalln(args ...interface{}) {
	fmt.Println(args...)
	os.Exit(1)
}

// This ties together a single reflex 'instance' so that multiple watches/commands can be handled together
// easily.
type Reflex struct {
	id        int
	start     bool
	backlog   Backlog
	regex     *regexp.Regexp
	glob      string
	useRegex  bool
	onlyFiles bool
	onlyDirs  bool
	command   []string
	subSymbol string

	done       chan error
	rawChanges chan string
	filtered   chan string
	batched    chan string
}

// This function is not threadsafe.
func NewReflex(c *Config, command []string) (*Reflex, error) {
	regex, glob, err := parseMatchers(c.regex, c.glob)
	if err != nil {
		Fatalln("Error parsing glob/regex.\n" + err.Error())
	}
	if len(command) == 0 {
		return nil, errors.New("Must give command to execute.")
	}

	if c.subSymbol == "" {
		return nil, errors.New("Substitution symbol must be non-empty.")
	}

	substitution := false
	for _, part := range command {
		if strings.Contains(part, c.subSymbol) {
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

	if c.onlyFiles && c.onlyDirs {
		return nil, errors.New("Cannot specify both --only-files and --only-dirs.")
	}

	reflex := &Reflex{
		id:        reflexID,
		start:     c.start,
		backlog:   backlog,
		regex:     regex,
		glob:      glob,
		useRegex:  regex != nil,
		onlyFiles: c.onlyFiles,
		onlyDirs:  c.onlyDirs,
		command:   command,
		subSymbol: c.subSymbol,

		rawChanges: make(chan string),
		filtered:   make(chan string),
		batched:    make(chan string),
	}
	reflexID++

	return reflex, nil
}

func (r *Reflex) PrintInfo(source string) {
	fmt.Println("Reflex from", source)
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
	fmt.Println("| Substitution symbol", r.subSymbol)
	replacer := strings.NewReplacer(r.subSymbol, "<filaname>")
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

func main() {
	if err := globalFlags.Parse(os.Args[1:]); err != nil {
		Fatalln(err)
	}
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

	reflexes := []*Reflex{}

	if flagConf == "" {
		reflex, err := NewReflex(globalConfig, globalFlags.Args())
		if err != nil {
			Fatalln(err)
		}
		if verbose {
			reflex.PrintInfo("commandline")
		}
		reflexes = append(reflexes, reflex)
	} else {
		if anyNonGlobalsRegistered() {
			Fatalln("Cannot set other flags along with --config other than --sequential, --verbose, and --decoration.")
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
			reflex, err := NewReflex(config, flags.Args())
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

	stdout := make(chan OutMsg, 100)
	stderr := make(chan OutMsg, 100)
	go printOutput(stdout, os.Stdout)
	go printOutput(stderr, os.Stderr)

	for _, reflex := range reflexes {
		go filterMatching(reflex.rawChanges, reflex.filtered, reflex)
		go batchRun(reflex.filtered, reflex.batched, reflex.backlog)
		go runEach(reflex.batched, stdout, stderr, reflex)
	}

	Fatalln(<-done)
}
