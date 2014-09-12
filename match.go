package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// A MatchingFxn decides whether some filename matches its set of patterns.
type MatchingFxn func(name string) bool

// matchAll is an all-accepting MatchingFxn.
func matchAll(name string) bool { return true }

func globMatcher(gs string, invert bool) (MatchingFxn, error) {
	return func(name string) bool {
		matches, err := filepath.Match(gs, name)
		// TODO: It would be good to notify the user on an error here.
		if err != nil {
			infoPrintln(0, "Error matching glob:", err)
			return false
		}
		return (invert != matches) // XOR
	}, nil
}

func regexMatcher(rs string, invert bool) (MatchingFxn, error) {
	regex, err := regexp.Compile(rs)
	if err != nil {
		return nil, err
	}
	return func(name string) bool {
		return (invert != regex.MatchString(name)) // XOR
	}, nil
}

type PatternInvertPair struct {
	pattern      string
	invertString string
}

func ParseMatchers(rs, gs, invert_rs, invert_gs string) (matcher MatchingFxn, matcherInfo string, err error) {
	if rs == "" && gs == "" && invert_rs == "" && invert_gs == "" {
		return matchAll, "| No regex (-r|-R) or glob (-g|-G) given, so matching all files", nil
	}

	var matchers = []MatchingFxn{}
	var matchInfos = []string{}
	for _, x := range []PatternInvertPair{{gs, ""}, {invert_gs, "invert"}} {
		if x.pattern != "" {
			m, err := globMatcher(x.pattern, x.invertString != "")
			if err != nil {
				return nil, "", err
			}
			matchers = append(matchers, m)
			matchInfos = append(matchInfos, fmt.Sprintf("| %sGlob: %s", x.invertString, x.pattern))
		}
	}
	for _, x := range []PatternInvertPair{{rs, ""}, {invert_rs, "invert"}} {
		if x.pattern != "" {
			m, err := regexMatcher(x.pattern, x.invertString != "")
			if err != nil {
				return nil, "", err
			}
			matchers = append(matchers, m)
			matchInfos = append(matchInfos, fmt.Sprintf("| %sRegex: %s", x.invertString, x.pattern))
		}
	}
	return func(name string) bool {
		for _, m := range matchers {
			if !m(name) {
				return false
			}
		}
		return true
	}, strings.Join(matchInfos, ", and\n"), nil
}
