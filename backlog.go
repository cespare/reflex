package main

// A Backlog represents a backlog of file paths that may be received while we're still running a command.
// There are a couple of different policies for how to handle this. If there are no {} (substitution
// sequences) in the command, then we only need to preserve one of the paths. If there is a {}, then we need
// to preserve each unique path in the backlog.
type Backlog interface {
	// Add a path to the backlog
	Add(path string)
	// Show what path should be processed next (without removing it from the backlog).
	Next() string
	// Remove the next path from the backlog and returns whether the backlog is now empty.
	RemoveOne() (empty bool)
}

type UnifiedBacklog string

func (b *UnifiedBacklog) Add(path string) {
	if b == nil {
		*b = UnifiedBacklog(path)
	}
}

func (b *UnifiedBacklog) Next() string {
	if b == nil {
		panic("Next() called on empty backlog")
	}
	return string(*b)
}

func (b *UnifiedBacklog) RemoveOne() bool {
	if b == nil {
		panic("RemoveOne() called on empty backlog")
	}
	b = nil
	return true
}

type UniqueFilesBacklog struct {
	empty bool
	next  string
	rest  map[string]struct{}
}

func (b *UniqueFilesBacklog) Add(path string) {
	if path == b.next {
		return
	}
	b.rest[path] = struct{}{}
	b.empty = false
}

func (b *UniqueFilesBacklog) Next() string {
	if b.empty {
		panic("Next() called on empty backlog")
	}
	return b.next
}

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
