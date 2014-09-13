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
	m = &globMatcher{glob: "foo*", invert: true}
	equals(t, false, m.Match("foo"))
	equals(t, false, m.Match("foobar"))
	equals(t, true, m.Match("bar"))
}

func TestRegexMatcher(t *testing.T) {
	m := &regexMatcher{regex: regexp.MustCompile("foo.*")}
	equals(t, true, m.Match("foo"))
	equals(t, true, m.Match("foobar"))
	equals(t, false, m.Match("bar"))
	m = &regexMatcher{regex: regexp.MustCompile("foo.*"), invert: true}
	equals(t, false, m.Match("foo"))
	equals(t, false, m.Match("foobar"))
	equals(t, true, m.Match("bar"))
}

func TestMultiMatcher(t *testing.T) {
	m := multiMatcher{
		&regexMatcher{regex: regexp.MustCompile("foo")},
		&regexMatcher{regex: regexp.MustCompile(`\.go$`)},
		&regexMatcher{regex: regexp.MustCompile("foobar"), invert: true},
	}
	equals(t, true, m.Match("foo.go"))
	equals(t, true, m.Match("foo/bar.go"))
	equals(t, false, m.Match("foobar/blah.go"))
}
