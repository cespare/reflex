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
	"time"

	"github.com/howeyc/fsnotify"
)

const (
	subSymbol = "{}"
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
			path := strings.TrimPrefix(e.Name, "./")
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

// runCommand runs the specified command, replacing {} in args with the provided filename. Blocks until the
// command exits.
func runCommand(command []string, path string) {
	replacer := strings.NewReplacer(subSymbol, path)
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

// filterMatching passes on messages matching regex.
func filterMatching(in <-chan string, out chan<- string, regex *regexp.Regexp) {
	for name := range in {
		if regex.MatchString(name) {
			out <- name
		}
	}
}

// batchRun receives realtime file notification events and batches them up. It's a bit tricky, but here's what
// it accomplishes:
// * When we initially get a message, wait a bit and batch messages before trying to send anything. This is
//	 because the file events come in quick bursts.
// * Once it's time to send, don't do it until the out channel is unblocked. In the meantime, keep batching.
//   When we've sent off all the batched messages, go back to the beginning.
func batchRun(in <-chan string, out chan<- string, backlog Backlog) {
	for name := range in {
		backlog.Add(name)
		timer := time.NewTimer(500 * time.Millisecond)
	outer:
		for {
			select {
			case name := <-in:
				backlog.Add(name)
			case <-timer.C:
				for {
					select {
					case name := <-in:
						backlog.Add(name)
					case out <- backlog.Next():
						if backlog.RemoveOne() {
							break outer
						}
					}
				}
			}
		}
	}
}

// runEach runs the command on each name that comes through the names channel.
func runEach(names <-chan string, command []string) {
	for name := range names {
		runCommand(command, name)
	}
}

func main() {
	flag.Parse()
	regex := parseRegex()
	command := flag.Args()
	if len(command) == 0 {
		Fatalln("Must give command to execute.")
	}
	substitution := false
	for _, part := range command {
		if strings.Contains(part, subSymbol) {
			substitution = true
			break
		}
	}
	var backlog Backlog
	if substitution {
		backlog = &UniqueFilesBacklog{true, "", make(map[string]struct{})}
	} else {
		backlog = new(UnifiedBacklog)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		Fatalln(err)
	}
	defer watcher.Close()

	done := make(chan error)
	rawChanges := make(chan string)
	filtered := make(chan string)
	batched := make(chan string)

	go watch(".", watcher, rawChanges, done)
	go filterMatching(rawChanges, filtered, regex)
	go batchRun(filtered, batched, backlog)
	go runEach(batched, command)

	Fatalln(<-done)
}
