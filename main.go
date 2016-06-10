package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	flag "github.com/ogier/pflag"
	"gopkg.in/fsnotify.v1"
)

const defaultSubSymbol = "{}"

var (
	reflexes []*Reflex

	flagConf       string
	flagSequential bool
	flagDecoration string
	decoration     Decoration
	verbose        bool
	chmods         bool
	globalFlags    = flag.NewFlagSet("", flag.ContinueOnError)
	globalConfig   = &Config{}

	reflexID = 0
	stdout   = make(chan OutMsg, 1)

	cleanupMu = &sync.Mutex{}
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
	globalFlags.BoolVarP(&chmods, "chmods", "m", false, `
            Includes watching chmod *only* events.`)
	globalFlags.BoolVarP(&flagSequential, "sequential", "e", false, `
            Don't run multiple commands at the same time.`)
	globalFlags.StringVarP(&flagDecoration, "decoration", "d", "plain", `
            How to decorate command output. Choices: none, plain, fancy.`)
	globalConfig.registerFlags(globalFlags)
}

func anyNonGlobalsRegistered() bool {
	any := false
	walkFn := func(f *flag.Flag) {
		if !(f.Name == "config" || f.Name == "verbose" || f.Name == "sequential" || f.Name == "decoration"|| f.Name == "chmods") {
			any = any || true
		}
	}
	globalFlags.Visit(walkFn)
	return any
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
	cleanupMu.Lock()
	fmt.Println(reason)
	wg := &sync.WaitGroup{}
	for _, reflex := range reflexes {
		if reflex.Running() {
			wg.Add(1)
			go func(reflex *Reflex) {
				reflex.terminate()
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
			Fatalln("Cannot set other flags along with --config other than --sequential, --verbose, --decoration, and --chmods.")
		}
		var err error
		configs, err = ReadConfigs(flagConf)
		if err != nil {
			Fatalln("Could not parse configs: ", err)
		}
		if len(configs) == 0 {
			Fatalln("No configurations found")
		}
	}

	for _, config := range configs {
		reflex, err := NewReflex(config)
		if err != nil {
			Fatalln("Could not make reflex for config:", err)
		}
		if verbose {
			fmt.Println(reflex)
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

	changes := make(chan string)
	broadcastChanges := make([]chan string, len(reflexes))
	done := make(chan error)
	for i := range reflexes {
		broadcastChanges[i] = make(chan string)
	}
	go watch(".", watcher, changes, done, reflexes)
	go broadcast(broadcastChanges, changes)
	go printOutput(stdout, os.Stdout)

	for i, reflex := range reflexes {
		reflex.Start(broadcastChanges[i])
	}

	Fatalln(<-done)
}

func broadcast(outs []chan string, in <-chan string) {
	for e := range in {
		for _, out := range outs {
			out <- e
		}
	}
}
