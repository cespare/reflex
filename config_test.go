package main

import (
	"strings"
	"testing"
)

func TestReadConfigs(t *testing.T) {
	const in = `-g '*.go' echo {}

# Some comment here
-r '^a[0-9]+\.txt$' --only-dirs --substitute='[]' echo []
-g '*.go' -s --only-files echo hi
-r foo -r bar -R baz -g a -G b -G c echo hi`

	configs, err := readConfigsFromReader(strings.NewReader(in), "test input")
	ok(t, err)
	exp := []*Config{
		{
			command:   []string{"echo", "{}"},
			source:    "test input, line 1",
			globs:     []string{"*.go"},
			subSymbol: "{}",
		},
		{
			command:   []string{"echo", "[]"},
			source:    "test input, line 4",
			regexes:   []string{`^a[0-9]+\.txt$`},
			subSymbol: "[]",
			onlyDirs:  true,
		},
		{
			command:      []string{"echo", "hi"},
			source:       "test input, line 5",
			globs:        []string{"*.go"},
			subSymbol:    "{}",
			startService: true,
			onlyFiles:    true,
		},
		{
			command:        []string{"echo", "hi"},
			source:         "test input, line 6",
			regexes:        []string{"foo", "bar"},
			globs:          []string{"a"},
			inverseRegexes: []string{"baz"},
			inverseGlobs:   []string{"b", "c"},
			subSymbol:      "{}",
		},
	}
	equals(t, exp, configs)
}

func TestReadConfigsBad(t *testing.T) {
	for _, in := range []string{
		"",
		"--abc echo hi",
		"-g '*.go'",
		"--substitute='' echo hi",
		"-s echo {}",
		"--only-files --only-dirs echo hi",
	} {
		configs, err := readConfigsFromReader(strings.NewReader(in), "test input")
		if err == nil {
			for _, config := range configs {
				_, err := NewReflex(config)
				assert(t, err != nil, "expected nil; got %#v (%q)", err, in)
			}
		}
	}
}
