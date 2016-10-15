package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kr/pretty"
)

func TestReadConfigs(t *testing.T) {
	const in = `-g '*.go' echo {}

# Some comment here
-r '^a[0-9]+\.txt$' --only-dirs --substitute='[]' echo []
-g '*.go' -s --only-files echo hi
-r foo -r bar -R baz -g a -G b -G c echo hi
`

	got, err := readConfigsFromReader(strings.NewReader(in), "test input")
	if err != nil {
		t.Fatal(err)
	}
	want := []*Config{
		{
			command:         []string{"echo", "{}"},
			source:          "test input, line 1",
			globs:           []string{"*.go"},
			subSymbol:       "{}",
			shutdownTimeout: 500 * time.Millisecond,
		},
		{
			command:         []string{"echo", "[]"},
			source:          "test input, line 4",
			regexes:         []string{`^a[0-9]+\.txt$`},
			subSymbol:       "[]",
			shutdownTimeout: 500 * time.Millisecond,
			onlyDirs:        true,
		},
		{
			command:         []string{"echo", "hi"},
			source:          "test input, line 5",
			globs:           []string{"*.go"},
			subSymbol:       "{}",
			startService:    true,
			shutdownTimeout: 500 * time.Millisecond,
			onlyFiles:       true,
		},
		{
			command:         []string{"echo", "hi"},
			source:          "test input, line 6",
			regexes:         []string{"foo", "bar"},
			globs:           []string{"a"},
			inverseRegexes:  []string{"baz"},
			inverseGlobs:    []string{"b", "c"},
			subSymbol:       "{}",
			shutdownTimeout: 500 * time.Millisecond,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("readConfigsFromReader: got diffs:\n%s",
			strings.Join(pretty.Diff(got, want), "\n"))
	}
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
		r := strings.NewReader(in)
		if configs, err := readConfigsFromReader(r, "test input"); err == nil {
			for _, config := range configs {
				if _, err := NewReflex(config); err == nil {
					t.Errorf("readConfigsFromReader(%q): got nil error")
				}
			}
		}
	}
}
