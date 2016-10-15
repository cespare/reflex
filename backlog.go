package main

// A Backlog represents a queue of file paths that may be received while we're
// still running a command. There are a couple of different policies for how to
// handle this. If there are no {} (substitution sequences) in the command, then
// we only need to preserve one of the paths. If there is a {}, then we need to
// preserve each unique path in the backlog.
type Backlog interface {
	// Add a path to the backlog.
	Add(path string)
	// Show what path should be processed next.
	Next() string
	// Remove the next path from the backlog and return whether
	// the backlog is now empty.
	RemoveOne() (empty bool)
}

// A UnifiedBacklog only remembers one backlog item at a time.
type UnifiedBacklog struct {
	s     string
	empty bool
}

func NewUnifiedBacklog() *UnifiedBacklog {
	return &UnifiedBacklog{empty: true}
}

// Add adds path to b if there is not a path there currently.
// Otherwise it discards it.
func (b *UnifiedBacklog) Add(path string) {
	if b.empty {
		b.s = path
		b.empty = false
	}
}

// Next returns the path in b.
func (b *UnifiedBacklog) Next() string {
	if b.empty {
		panic("Next() called on empty backlog")
	}
	return b.s
}

// RemoveOne removes the path in b.
func (b *UnifiedBacklog) RemoveOne() bool {
	if b.empty {
		panic("RemoveOne() called on empty backlog")
	}
	b.empty = true
	b.s = ""
	return true
}

// A UniqueFilesBacklog keeps a set of the paths it has received.
type UniqueFilesBacklog struct {
	empty bool
	next  string
	rest  map[string]struct{}
}

func NewUniqueFilesBacklog() *UniqueFilesBacklog {
	return &UniqueFilesBacklog{
		empty: true,
		next:  "",
		rest:  make(map[string]struct{}),
	}
}

// Add adds path to the set of files in b.
func (b *UniqueFilesBacklog) Add(path string) {
	defer func() { b.empty = false }()
	if b.empty {
		b.next = path
		return
	}
	if path == b.next {
		return
	}
	b.rest[path] = struct{}{}
}

// Next returns one of the paths in b.
func (b *UniqueFilesBacklog) Next() string {
	if b.empty {
		panic("Next() called on empty backlog")
	}
	return b.next
}

// RemoveOne removes one of the paths from b (the same path that was returned by
// a preceding call to Next).
func (b *UniqueFilesBacklog) RemoveOne() bool {
	if b.empty {
		panic("RemoveOne() called on empty backlog")
	}
	if len(b.rest) == 0 {
		b.next = ""
		b.empty = true
		return true
	}
	for next := range b.rest {
		b.next = next
		break
	}
	delete(b.rest, b.next)
	return false
}
