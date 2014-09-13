package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// A Matcher decides whether some filename matches its set of patterns.
type Matcher interface {
	Match(name string) bool
	String() string
}

// matchAll is an all-accepting Matcher.
type matchAll struct{}

func (matchAll) Match(name string) bool { return true }
func (matchAll) String() string         { return "(Implicitly matching all non-excluded files)" }

type globMatcher struct {
	glob   string
	invert bool
}

func (m *globMatcher) Match(name string) bool {
	matches, err := filepath.Match(m.glob, name)
	if err != nil {
		return false
	}
	return matches != m.invert
}

func (m *globMatcher) String() string {
	s := "Glob"
	if m.invert {
		s = "Inverted glob"
	}
	return fmt.Sprintf("%s match: %q", s, m.glob)
}

type regexMatcher struct {
	regex  *regexp.Regexp
	invert bool
}

func (m *regexMatcher) Match(name string) bool {
	return m.regex.MatchString(name) != m.invert
}

func (m *regexMatcher) String() string {
	s := "Regex"
	if m.invert {
		s = "Inverted regex"
	}
	return fmt.Sprintf("%s match: %q", s, m.regex.String())
}

// A multiMatcher returns the logical AND of its sub-matchers.
type multiMatcher []Matcher

func (m multiMatcher) Match(name string) bool {
	for _, matcher := range m {
		if !matcher.Match(name) {
			return false
		}
	}
	return true
}

func (m multiMatcher) String() string {
	var s []string
	for _, matcher := range m {
		s = append(s, matcher.String())
	}
	return strings.Join(s, "\n")
}

func ParseMatchers(regexes, inverseRegexes, globs, inverseGlobs []string) (m Matcher, err error) {
	var matchers multiMatcher
	if len(regexes) == 0 && len(globs) == 0 {
		matchers = multiMatcher{matchAll{}}
	}
	for _, r := range regexes {
		regex, err := regexp.Compile(r)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, &regexMatcher{
			regex:  regex,
			invert: false,
		})
	}
	for _, r := range inverseRegexes {
		regex, err := regexp.Compile(r)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, &regexMatcher{
			regex:  regex,
			invert: true,
		})
	}
	for _, g := range globs {
		matchers = append(matchers, &globMatcher{
			glob:   g,
			invert: false,
		})
	}
	for _, g := range inverseGlobs {
		matchers = append(matchers, &globMatcher{
			glob:   g,
			invert: true,
		})
	}
	return matchers, nil
}
