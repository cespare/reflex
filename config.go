package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/kballard/go-shellquote"
	flag "github.com/ogier/pflag"
)

type Config struct {
	command         []string
	source          string
	regexes         []string
	globs           []string
	inverseRegexes  []string
	inverseGlobs    []string
	subSymbol       string
	startService    bool
	shutdownTimeout time.Duration
	onlyFiles       bool
	onlyDirs        bool
	allFiles        bool
}

func (c *Config) registerFlags(f *flag.FlagSet) {
	f.VarP(newMultiString(nil, &c.regexes), "regex", "r", `
            A regular expression to match filenames. (May be repeated.)`)
	f.VarP(newMultiString(nil, &c.inverseRegexes), "inverse-regex", "R", `
            A regular expression to exclude matching filenames.
            (May be repeated.)`)
	f.VarP(newMultiString(nil, &c.globs), "glob", "g", `
            A shell glob expression to match filenames. (May be repeated.)`)
	f.VarP(newMultiString(nil, &c.inverseGlobs), "inverse-glob", "G", `
            A shell glob expression to exclude matching filenames.
            (May be repeated.)`)
	f.StringVar(&c.subSymbol, "substitute", defaultSubSymbol, `
            The substitution symbol that is replaced with the filename
            in a command.`)
	f.BoolVarP(&c.startService, "start-service", "s", false, `
            Indicates that the command is a long-running process to be
            restarted on matching changes.`)
	f.DurationVarP(&c.shutdownTimeout, "shutdown-timeout", "t", 500*time.Millisecond, `
            Allow services this long to shut down.`)
	f.BoolVar(&c.onlyFiles, "only-files", false, `
            Only match files (not directories).`)
	f.BoolVar(&c.onlyDirs, "only-dirs", false, `
            Only match directories (not files).`)
	f.BoolVar(&c.allFiles, "all", false, `
            Include normally ignored files (VCS and editor special files).`)
}

// ReadConfigs reads configurations from either a file or, as a special case,
// stdin if "-" is given for path.
func ReadConfigs(path string) ([]*Config, error) {
	var r io.Reader
	name := path
	if path == "-" {
		r = os.Stdin
		name = "standard input"
	} else {
		f, err := os.Open(flagConf)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	return readConfigsFromReader(r, name)
}

func readConfigsFromReader(r io.Reader, name string) ([]*Config, error) {
	scanner := bufio.NewScanner(r)
	lineNo := 0
	var configs []*Config
	for scanner.Scan() {
		lineNo++
		errorf := fmt.Sprintf("error on line %d of %s: %%s", lineNo, name)
		c := &Config{}
		flags := flag.NewFlagSet("", flag.ContinueOnError)
		flags.SetOutput(ioutil.Discard)
		c.registerFlags(flags)
		parts, err := shellquote.Split(scanner.Text())
		if err != nil {
			return nil, fmt.Errorf(errorf, err)
		}
		// Skip empty lines and comments (lines starting with #).
		if len(parts) == 0 || strings.HasPrefix(parts[0], "#") {
			continue
		}
		if err := flags.Parse(parts); err != nil {
			return nil, fmt.Errorf(errorf, err)
		}
		c.command = flags.Args()
		c.source = fmt.Sprintf("%s, line %d", name, lineNo)
		configs = append(configs, c)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config from %s: %s", name, err)
	}
	return configs, nil
}

// A multiString is a flag.Getter which collects repeated string flags.
type multiString struct {
	vals *[]string
	set  bool // If false, then vals contains the defaults.
}

func newMultiString(vals []string, p *[]string) *multiString {
	*p = vals
	return &multiString{vals: p}
}

func (s *multiString) Set(val string) error {
	if s.set {
		*s.vals = append(*s.vals, val)
	} else {
		*s.vals = []string{val}
		s.set = true
	}
	return nil
}

func (s *multiString) Get() interface{} {
	return s.vals
}

func (s *multiString) String() string {
	return fmt.Sprintf("[%s]", strings.Join(*s.vals, " "))
}
