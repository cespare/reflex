package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"strings"
	"sync"
)

// A Matcher decides whether some filename matches its set of patterns.
type Matcher interface {
	// Match returns whether a filename matches.
	Match(name string) bool
	// ExcludePrefix returns whether all paths with this prefix cannot match.
	// It is allowed to return false negatives but not false positives.
	// This is used as an optimization for skipping directory watches with
	// inverted matches.
	ExcludePrefix(prefix string) bool
	String() string
}

// ParseMatchers combines multiple (possibly inverse) regex and glob patterns
// into a single Matcher.
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
		matchers = append(matchers, newRegexMatcher(regex, false))
	}
	for _, r := range inverseRegexes {
		regex, err := regexp.Compile(r)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, newRegexMatcher(regex, true))
	}
	for _, g := range globs {
		matchers = append(matchers, &globMatcher{glob: g})
	}
	for _, g := range inverseGlobs {
		matchers = append(matchers, &globMatcher{
			glob:    g,
			inverse: true,
		})
	}
	return matchers, nil
}

// matchAll is an all-accepting Matcher.
type matchAll struct{}

func (matchAll) Match(name string) bool           { return true }
func (matchAll) ExcludePrefix(prefix string) bool { return false }
func (matchAll) String() string                   { return "(Implicitly matching all non-excluded files)" }

type globMatcher struct {
	glob    string
	inverse bool
}

func (m *globMatcher) Match(name string) bool {
	matches, err := filepath.Match(m.glob, name)
	if err != nil {
		return false
	}
	return matches != m.inverse
}

func (m *globMatcher) ExcludePrefix(prefix string) bool { return false }

func (m *globMatcher) String() string {
	s := "Glob"
	if m.inverse {
		s = "Inverted glob"
	}
	return fmt.Sprintf("%s match: %q", s, m.glob)
}

type regexMatcher struct {
	regex   *regexp.Regexp
	inverse bool

	mu               *sync.Mutex // protects following
	canExcludePrefix bool        // This regex has no $, \z, or \b -- see ExcludePrefix
	excludeChecked   bool
}

func (m *regexMatcher) Match(name string) bool {
	return m.regex.MatchString(name) != m.inverse
}

func newRegexMatcher(regex *regexp.Regexp, inverse bool) *regexMatcher {
	return &regexMatcher{
		regex:   regex,
		inverse: inverse,
		mu:      new(sync.Mutex),
	}
}

// ExcludePrefix returns whether this matcher cannot possibly match any path
// with a particular prefix. The question is: given a regex r and some prefix p
// which r accepts, is there any string s that has p as a prefix that r does not
// accept?
//
// With a classic regular expression from CS, this can only be the case if r
// ends with $, the end-of-input token (because once the NFA is in an accepting
// state, adding more input will not change that). In Go's regular expressions,
// I think the only way to construct a regex that would not meet this criteria
// is by using zero-width lookahead. There is no arbitrary lookahead in Go, so
// the only zero-width lookahead is provided by $, \z, and \b. For instance, the
// following regular expressions match the "foo", but not "foobar":
//
//   foo$
//   foo\b
//   (foo$)|(baz$)
//
// Thus, to choose whether we can exclude this prefix, m must be an inverse
// matcher that does not contain the zero-width ops $, \z, and \b.
func (m *regexMatcher) ExcludePrefix(prefix string) bool {
	if !m.inverse {
		return false
	}
	if !m.regex.MatchString(prefix) || m.regex.String() == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.excludeChecked {
		r, err := syntax.Parse(m.regex.String(), syntax.Perl)
		if err != nil {
			panic("Cannot compile regex, but it was previously compiled!?!")
		}
		r = r.Simplify()
		stack := []*syntax.Regexp{r}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch cur.Op {
			case syntax.OpEndLine, syntax.OpEndText, syntax.OpWordBoundary:
				m.canExcludePrefix = false
				goto after
			}
			if cur.Sub0[0] != nil {
				stack = append(stack, cur.Sub0[0])
			}
			stack = append(stack, cur.Sub...)
		}
		m.canExcludePrefix = true
	after:
		m.excludeChecked = true
	}
	return m.canExcludePrefix
}

func (m *regexMatcher) String() string {
	s := "Regex"
	if m.inverse {
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

func (m multiMatcher) ExcludePrefix(prefix string) bool {
	for _, matcher := range m {
		if matcher.ExcludePrefix(prefix) {
			return true
		}
	}
	return false
}

func (m multiMatcher) String() string {
	var s []string
	for _, matcher := range m {
		s = append(s, matcher.String())
	}
	return strings.Join(s, "\n")
}
