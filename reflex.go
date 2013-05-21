package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/howeyc/fsnotify"
)

var (
	regexString string
	matchAll    = regexp.MustCompile(".*")
)

func init() {
	flag.StringVar(&regexString, "r", "", "The regular expression to match filenames.")
}

func parseRegex() *regexp.Regexp {
	if regexString == "" {
		fmt.Println("Warning: no regex given (-r), so matching all file changes.")
		return matchAll
	}

	r, err := regexp.Compile(regexString)
	if err != nil {
		Fatalln("Bad regular expression provided.\n" + err.Error())
	}
	return r
}

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

func runCommand(command []string, path string) {
	replacer := strings.NewReplacer("{}", path)
	args := make([]string, len(command))
	for i, c := range command {
		args[i] = replacer.Replace(c)
	}

	cmd := exec.Command(args[0], args[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("Error running command:", err)
		return
	}
	go io.Copy(os.Stdout, stdout)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("Error running command:", err)
		return
	}
	go io.Copy(os.Stderr, stderr)

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "(error exit: %s)\n", err)
	}
}

func runMatching(names <-chan string, regex *regexp.Regexp, command []string) {
	for name := range names {
		normalized := strings.TrimPrefix(name, "./")
		if !regex.MatchString(normalized) {
			continue
		}
		go runCommand(command, normalized)
	}
}

func main() {
	flag.Parse()
	regex := parseRegex()
	command := flag.Args()
	if len(command) == 0 {
		Fatalln("Must give command to execute.")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		Fatalln(err)
	}
	defer watcher.Close()

	done := make(chan error)
	rawChanges := make(chan string)

	go watch(".", watcher, rawChanges, done)
	go runMatching(rawChanges, regex, command)

	Fatalln(<-done)
}
