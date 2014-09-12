package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	flag "github.com/cespare/pflag"
	"github.com/kballard/go-shellquote"
)

type Config struct {
	command []string
	source  string

	regex        string
	glob         string
	subSymbol    string
	startService bool
	onlyFiles    bool
	onlyDirs     bool
}

func (c *Config) registerFlags(f *flag.FlagSet) {
	f.StringVarP(&c.regex, "regex", "r", "", `
            A regular expression to match filenames.`)
	f.StringVarP(&c.glob, "glob", "g", "", `
            A shell glob expression to match filenames.`)
	f.StringVar(&c.subSymbol, "substitute", defaultSubSymbol, `
            The substitution symbol that is replaced with the filename
            in a command.`)
	f.BoolVarP(&c.startService, "start-service", "s", false, `
            Indicates that the command is a long-running process to be
            restarted on matching changes.`)
	f.BoolVar(&c.onlyFiles, "only-files", false, `
            Only match files (not directories).`)
	f.BoolVar(&c.onlyDirs, "only-dirs", false, `
            Only match directories (not files).`)
}

// ReadConfigs reads configurations from either a file or, as a special case, stdin if "-" is given for path.
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

	scanner := bufio.NewScanner(r)
	lineNo := 0
	var configs []*Config
	for scanner.Scan() {
		lineNo++
		errorf := fmt.Sprintf("error on line %d of %s: %%s", lineNo, name)
		c := &Config{}
		flags := flag.NewFlagSet("", flag.ContinueOnError)
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
