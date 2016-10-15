package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

const chmodMask fsnotify.Op = ^fsnotify.Op(0) ^ fsnotify.Chmod

// watch recursively watches changes in root and reports the filenames to names.
// It sends an error on the done chan.
// As an optimization, any dirs we encounter that meet the ExcludePrefix
// criteria of all reflexes can be ignored.
func watch(root string, watcher *fsnotify.Watcher, names chan<- string, done chan<- error, reflexes []*Reflex) {
	if err := filepath.Walk(root, walker(watcher, reflexes)); err != nil {
		infoPrintf(-1, "Error while walking path %s: %s", root, err)
	}

	for {
		select {
		case e := <-watcher.Events:
			if verbose {
				infoPrintln(-1, "fsnotify event:", e)
			}
			stat, err := os.Stat(e.Name)
			if err != nil {
				continue
			}
			path := normalize(e.Name, stat.IsDir())
			if e.Op&chmodMask == 0 {
				continue
			}
			names <- path
			if e.Op&fsnotify.Create > 0 && stat.IsDir() {
				if err := filepath.Walk(path, walker(watcher, reflexes)); err != nil {
					infoPrintf(-1, "Error while walking path %s: %s", path, err)
				}
			}
			// TODO: Cannot currently remove fsnotify watches
			// recursively, or for deleted files. See:
			// https://github.com/cespare/reflex/issues/13
			// https://github.com/go-fsnotify/fsnotify/issues/40
			// https://github.com/go-fsnotify/fsnotify/issues/41
		case err := <-watcher.Errors:
			done <- err
			return
		}
	}
}

func walker(watcher *fsnotify.Watcher, reflexes []*Reflex) filepath.WalkFunc {
	return func(path string, f os.FileInfo, err error) error {
		if err != nil || !f.IsDir() {
			return nil
		}
		path = normalize(path, f.IsDir())
		ignore := true
		for _, r := range reflexes {
			if !r.matcher.ExcludePrefix(path) {
				ignore = false
				break
			}
		}
		if ignore {
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			infoPrintf(-1, "Error while watching new path %s: %s", path, err)
		}
		return nil
	}
}

func normalize(path string, dir bool) string {
	path = strings.TrimPrefix(path, "./")
	if dir && !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	return path
}
