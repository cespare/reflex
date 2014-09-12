package main

import (
	"sort"
	"testing"
)

func TestUnifiedBacklog(t *testing.T) {
	b := NewUnifiedBacklog()
	b.Add("foo")
	b.Add("bar")
	equals(t, "foo", b.Next())
	equals(t, true, b.RemoveOne())
	panics(t, b.Next)
	panics(t, b.RemoveOne)
}

func TestUniqueFilesBacklog(t *testing.T) {
	b := NewUniqueFilesBacklog()
	b.Add("foo")
	b.Add("bar")
	var s []string
	s = append(s, b.Next())
	equals(t, false, b.RemoveOne())
	s = append(s, b.Next())
	equals(t, true, b.RemoveOne())
	sort.Strings(s)
	equals(t, []string{"bar", "foo"}, s)
	panics(t, b.Next)
	panics(t, b.RemoveOne)
}
