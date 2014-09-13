package main

import (
	"regexp"
	"testing"
)

func TestGlobMatcher(t *testing.T) {
	m := &globMatcher{glob: "foo*"}
	equals(t, true, m.Match("foo"))
	equals(t, true, m.Match("foobar"))
	equals(t, false, m.Match("bar"))
	m = &globMatcher{glob: "foo*", inverse: true}
	equals(t, false, m.Match("foo"))
	equals(t, false, m.Match("foobar"))
	equals(t, true, m.Match("bar"))
}

func TestRegexMatcher(t *testing.T) {
	m := &regexMatcher{regex: regexp.MustCompile("foo.*")}
	equals(t, true, m.Match("foo"))
	equals(t, true, m.Match("foobar"))
	equals(t, false, m.Match("bar"))
	m = &regexMatcher{regex: regexp.MustCompile("foo.*"), inverse: true}
	equals(t, false, m.Match("foo"))
	equals(t, false, m.Match("foobar"))
	equals(t, true, m.Match("bar"))
}

func TestExcludePrefix(t *testing.T) {
	m := &regexMatcher{regex: regexp.MustCompile("foo")}
	equals(t, false, m.ExcludePrefix("bar")) // Never true for non-inverted

	for _, testCase := range []struct {
		re       string
		prefix   string
		expected bool
	}{
		{re: "foo", prefix: "foo", expected: true},
		{re: "((foo{3,4})|abc*)+|foo", prefix: "foo", expected: true},
		{re: "foo$", prefix: "foo", expected: false},
		{re: `foo\b`, prefix: "foo", expected: false},
		{re: `(foo\b)|(baz$)`, prefix: "foo", expected: false},
	} {
		m := &regexMatcher{regex: regexp.MustCompile(testCase.re), inverse: true}
		equals(t, testCase.expected, m.ExcludePrefix(testCase.prefix))
	}
}

func TestMultiMatcher(t *testing.T) {
	m := multiMatcher{
		&regexMatcher{regex: regexp.MustCompile("foo")},
		&regexMatcher{regex: regexp.MustCompile(`\.go$`)},
		&regexMatcher{regex: regexp.MustCompile("foobar"), inverse: true},
	}
	equals(t, true, m.Match("foo.go"))
	equals(t, true, m.Match("foo/bar.go"))
	equals(t, false, m.Match("foobar/blah.go"))
}
