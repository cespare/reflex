package main

import (
	"strings"
	"testing"
)

func TestReadConfigs(t *testing.T) {
	const in = `-g '*.go' echo {}

# Some comment here
-r '^a[0-9]+\.txt$' --only-dirs --substitute='[]' echo []
-g '*.go' -s --only-files echo hi`

	configs, err := readConfigsFromReader(strings.NewReader(in), "test input")
	ok(t, err)
	exp := []*Config{
		{
			command:      []string{"echo", "{}"},
			source:       "test input, line 1",
			regex:        "",
			glob:         "*.go",
			subSymbol:    "{}",
			startService: false,
			onlyFiles:    false,
			onlyDirs:     false,
		},
		{
			command:      []string{"echo", "[]"},
			source:       "test input, line 4",
			regex:        `^a[0-9]+\.txt$`,
			glob:         "",
			subSymbol:    "[]",
			startService: false,
			onlyFiles:    false,
			onlyDirs:     true,
		},
		{
			command:      []string{"echo", "hi"},
			source:       "test input, line 5",
			regex:        "",
			glob:         "*.go",
			subSymbol:    "{}",
			startService: true,
			onlyFiles:    true,
			onlyDirs:     false,
		},
	}
	equals(t, exp, configs)
}

func TestReadConfigsBad(t *testing.T) {
	for _, in := range []string{
		"",
		"-r a -g b echo hello",
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
				assert(t, err != nil, "expected nil; got %#v", err)
			}
		}
	}
}
