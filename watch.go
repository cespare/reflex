package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
)

const chmodMask fsnotify.Op = ^fsnotify.Op(0) ^ fsnotify.Chmod

// watch recursively watches changes in root and reports the filenames to names.
// It sends an error on the done chan.
// As an optimization, any dirs we encounter that meet the ExcludePrefix
// criteria of all reflexes can be ignored.

func walkerWithStatusCheck(root string, watcher *fsnotify.Watcher, reflexes []*Reflex) {
	if err := filepath.Walk(root, walker(watcher, reflexes)); err != nil {
		infoPrintf(-1, "Error while walking path %s: %s", root, err)
	}
	router := gin.Default()
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "READY",
		})
	})
	go router.Run(":9090")
	log.Println("Application is ready to hear new events and healthcheck is running on :9090/health")
}

func watch(root string, watcher *fsnotify.Watcher, names chan<- string, done chan<- error, reflexes []*Reflex) {
	walkerWithStatusCheck(root, watcher, reflexes)
	for {
		select {
		case e := <-watcher.Events:
			if verbose {
				infoPrintln(-1, "fsnotify event:", e)
			}
			stat, err := os.Stat(e.Name)
			if os.IsNotExist(err) {
				path := e.Name
				names <- path
			} else {
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
			}
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
