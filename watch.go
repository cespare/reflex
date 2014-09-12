package main

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/fsnotify.v1"
)

func walker(watcher *fsnotify.Watcher) filepath.WalkFunc {
	return func(path string, f os.FileInfo, err error) error {
		if err != nil || !f.IsDir() {
			return nil
		}
		if err := watcher.Add(path); err != nil {
			infoPrintf(-1, "Error while watching new path %s: %s", path, err)
		}
		return nil
	}
}

func watch(root string, watcher *fsnotify.Watcher, names chan<- string, done chan<- error) {
	if err := filepath.Walk(root, walker(watcher)); err != nil {
		infoPrintf(-1, "Error while walking path %s: %s", root, err)
	}

	for {
		select {
		case e := <-watcher.Events:
			path := strings.TrimPrefix(e.Name, "./")
			if verbose {
				infoPrintln(-1, "fsnotify event:", e)
			}
			if e.Op&fsnotify.Chmod > 0 {
				continue
			}
			names <- path
			if e.Op&fsnotify.Create > 0 {
				if err := filepath.Walk(path, walker(watcher)); err != nil {
					infoPrintf(-1, "Error while walking path %s: %s", path, err)
				}
			}
			// TODO: Cannot currently remove fsnotify watches recursively, or for deleted files. See:
			// https://github.com/cespare/reflex/issues/13
			// https://github.com/go-fsnotify/fsnotify/issues/40
			// https://github.com/go-fsnotify/fsnotify/issues/41
		case err := <-watcher.Errors:
			done <- err
			return
		}
	}
}
