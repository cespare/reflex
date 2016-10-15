package main

import (
	"reflect"
	"sort"
	"testing"
)

func TestUnifiedBacklog(t *testing.T) {
	b := NewUnifiedBacklog()
	b.Add("foo")
	b.Add("bar")
	if got, want := b.Next(), "foo"; got != want {
		t.Errorf("Next(): got %q; want %q", got, want)
	}
	if got := b.RemoveOne(); !got {
		t.Error("RemoveOne(): got !empty")
	}
}

func TestUniqueFilesBacklog(t *testing.T) {
	b := NewUniqueFilesBacklog()
	b.Add("foo")
	b.Add("bar")
	s := []string{b.Next()}
	if got := b.RemoveOne(); got {
		t.Error("RemoveOne(): got empty")
	}
	s = append(s, b.Next())
	if got := b.RemoveOne(); !got {
		t.Error("RemoveOne(): got !empty")
	}
	sort.Strings(s)
	if want := []string{"bar", "foo"}; !reflect.DeepEqual(s, want) {
		t.Errorf("Next() result set: got %v; want %v", s, want)
	}
}
