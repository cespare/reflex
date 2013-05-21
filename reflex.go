package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/howeyc/fsnotify"
)

func Fatalln(args ...interface{}) {
	fmt.Println(args...)
	os.Exit(1)
}

func walker(watcher *fsnotify.Watcher) filepath.WalkFunc {
	return func(path string, f os.FileInfo, err error) error {
		if !f.IsDir() {
			return nil
		}
		fmt.Println("Adding watch for path", path)
		if err := watcher.Watch(path); err != nil {
			// TODO: handle this somehow?
			fmt.Printf("Error while watching new path %s: %s\n", path, err)
		}
		return nil
	}
}

func watch(root string, watcher *fsnotify.Watcher, names chan<- string, done chan<- error) {
	if err := watcher.Watch(root); err != nil {
		Fatalln(err)
	}

	for {
		select {
		case e := <-watcher.Event:
			path := e.Name
			names <- path
			if e.IsCreate() {
				if err := filepath.Walk(path, walker(watcher)); err != nil {
					// TODO: handle this somehow?
					fmt.Printf("Error while walking path %s: %s\n", path, err)
				}
			}
			if e.IsDelete() {
				watcher.RemoveWatch(path)
			}
		case err := <-watcher.Error:
			done <- err
			return
		}
	}
}

func printNames(names <-chan string) {
	for name := range names {
		fmt.Println(name)
	}
}

func main() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		Fatalln(err)
	}
	defer watcher.Close()

	done := make(chan error)
	rawChanges := make(chan string)

	go watch(".", watcher, rawChanges, done)
	go printNames(rawChanges)

	Fatalln(<-done)
}
