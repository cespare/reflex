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
	m := newRegexMatcher(regexp.MustCompile("foo.*"), false)
	equals(t, true, m.Match("foo"))
	equals(t, true, m.Match("foobar"))
	equals(t, false, m.Match("bar"))
	m = newRegexMatcher(regexp.MustCompile("foo.*"), true)
	equals(t, false, m.Match("foo"))
	equals(t, false, m.Match("foobar"))
	equals(t, true, m.Match("bar"))
}

func TestExcludePrefix(t *testing.T) {
	m := newRegexMatcher(regexp.MustCompile("foo"), false)
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
		m := newRegexMatcher(regexp.MustCompile(testCase.re), true)
		equals(t, testCase.expected, m.ExcludePrefix(testCase.prefix))
	}
}

func TestMultiMatcher(t *testing.T) {
	m := multiMatcher{
		newRegexMatcher(regexp.MustCompile("foo"), false),
		newRegexMatcher(regexp.MustCompile(`\.go$`), false),
		newRegexMatcher(regexp.MustCompile("foobar"), true),
	}
	equals(t, true, m.Match("foo.go"))
	equals(t, true, m.Match("foo/bar.go"))
	equals(t, false, m.Match("foobar/blah.go"))
}

func TestDefaultExcludes(t *testing.T) {
	for _, testCase := range []struct {
		filename string
		expected bool
	}{
		{".git/HEAD", false},
		{"foo.git", true},
		{"foo/bar.git", true},
		{"foo/bar/.git/HEAD", false},
		{"foo~", false},
		{"foo/bar~", false},
		{"~foo", true},
		{"foo~bar", true},
		{"foo.swp", false},
		{"foo.swp.bar", true},
		{"foo/bar.swp", false},
		{"foo.#123", false},
		{"foo#123", true},
		{"foo/bar.#123", false},
		{"#foo#", false},
		{"foo/#bar#", false},
		{".DS_Store", false},
		{"foo/.DS_Store", false},
	} {
		exp := testCase.expected
		got := defaultExcludeMatcher.Match(testCase.filename)
		s := "was excluded"
		if !exp {
			s = "was not excluded"
		}
		assert(t, exp == got, "%q %s the default excludes matcher", testCase.filename, s)
	}
}
