package main

import (
	"regexp"
	"testing"
)

func TestMatchers(t *testing.T) {
	var (
		glob    = &globMatcher{glob: "foo*"}
		globInv = &globMatcher{glob: "foo*", inverse: true}

		regex    = newRegexMatcher(regexp.MustCompile("foo.*"), false)
		regexInv = newRegexMatcher(regexp.MustCompile("foo.*"), true)

		multi = multiMatcher{
			newRegexMatcher(regexp.MustCompile("foo"), false),
			newRegexMatcher(regexp.MustCompile(`\.go$`), false),
			newRegexMatcher(regexp.MustCompile("foobar"), true),
		}
	)
	for _, tt := range []struct {
		m    Matcher
		s    string
		want bool
	}{
		{glob, "foo", true},
		{glob, "foobar", true},
		{glob, "bar", false},
		{globInv, "foo", false},
		{globInv, "foobar", false},
		{globInv, "bar", true},

		{regex, "foo", true},
		{regex, "foobar", true},
		{regex, "bar", false},
		{regexInv, "foo", false},
		{regexInv, "foobar", false},
		{regexInv, "bar", true},

		{multi, "foo.go", true},
		{multi, "foo/bar.go", true},
		{multi, "foobar/blah.go", false},
	} {
		if got := tt.m.Match(tt.s); got != tt.want {
			t.Errorf("(%v).Match(%q): got %t; want %t",
				tt.m, tt.s, got, tt.want)
		}
	}
}

func TestExcludePrefix(t *testing.T) {
	m := newRegexMatcher(regexp.MustCompile("foo"), false)
	if m.ExcludePrefix("bar") {
		t.Error("m.ExcludePrefix gave true for a non-inverted matcher")
	}

	for _, tt := range []struct {
		re     string
		prefix string
		want   bool
	}{
		{"foo", "foo", true},
		{"((foo{3,4})|abc*)+|foo", "foo", true},
		{"foo$", "foo", false},
		{`foo\b`, "foo", false},
		{`(foo\b)|(baz$)`, "foo", false},
	} {
		m := newRegexMatcher(regexp.MustCompile(tt.re), true)
		if got := m.ExcludePrefix(tt.prefix); got != tt.want {
			t.Errorf("(%v).ExcludePrefix(%q): got %t; want %t",
				m, tt.prefix, got, tt.want)
		}
	}
}

func TestDefaultExcludes(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
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
		if got := defaultExcludeMatcher.Match(tt.name); got != tt.want {
			if got {
				t.Errorf("%q was excluded by the default excludes matcher", tt.name)
			} else {
				t.Errorf("%q was not excluded by the default excludes matcher", tt.name)
			}
		}
	}
}
