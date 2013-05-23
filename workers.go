package main

import (
	"bufio"
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

func walker(watcher *fsnotify.Watcher) filepath.WalkFunc {
	return func(path string, f os.FileInfo, err error) error {
		if err != nil || !f.IsDir() {
			// TODO: Is there some other thing we should be doing to handle errors? When watching large
			// directories that have lots of programs modifying them (esp. if they're making tempfiles along the
			// way), we often get errors.
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

func broadcast(in <-chan string, outs []chan<- string) {
	for e := range in {
		for _, out := range outs {
			out <- e
		}
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

// runCommand runs the given Command. Blocks until the command exits. All output is passed line-by-line to the
// stderr/stdout channels.
func runCommand(cmd *exec.Cmd, stdout chan<- string, stderr chan<- string) error {
	cmdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stdoutErr := make(chan error)
	go func() {
		scanner := bufio.NewScanner(cmdout)
		for scanner.Scan() {
			stdout <- scanner.Text()
		}
		stdoutErr <- scanner.Err()
	}()

	stderrErr := make(chan error)
	go func() {
		scanner := bufio.NewScanner(cmderr)
		for scanner.Scan() {
			stderr <- scanner.Text()
		}
		stderrErr <- scanner.Err()
	}()

	cmdErr := make(chan error)
	go func() {
		cmdErr <- cmd.Wait()
	}()

	for {
		select {
		case err := <-stdoutErr:
			if err != nil {
				return err
			}
			stdoutErr = nil
		case err := <-stderrErr:
			if err != nil {
				return err
			}
			stderrErr = nil
		case err := <-cmdErr:
			if err != nil {
				stderr <- fmt.Sprintf("(error exit: %s)\n", err)
			}
			cmdErr = nil
		}
		if stdoutErr == nil && stderrErr == nil && cmdErr == nil {
			break
		}
	}

	return nil
}

// filterMatchingRegex passes on messages matching regex.
func filterMatchingRegex(in <-chan string, out chan<- string, regex *regexp.Regexp) {
	for name := range in {
		if regex.MatchString(name) {
			out <- name
		}
	}
}

// filterMatchingGlob passes on messages matching glob.
func filterMatchingGlob(in <-chan string, out chan<- string, glob string) {
	for name := range in {
		matches, err := filepath.Match(glob, name)
		// TODO: It would be good to notify the user on an error here.
		if err == nil && matches {
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
		timer := time.NewTimer(200 * time.Millisecond)
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

// runEach runs the command on each name that comes through the names channel. Each {} is replaced by the name
// of the file. The stderr and stdout of the command are passed line-by-line to the stderr and stdout chans.
func runEach(names <-chan string, stdout chan<- string, stderr chan<- string, command []string, subSymbol string) {
	for name := range names {
		replacer := strings.NewReplacer(subSymbol, name)
		args := make([]string, len(command))
		for i, c := range command {
			args[i] = replacer.Replace(c)
		}
		cmd := exec.Command(args[0], args[1:]...)

		runCommand(cmd, stdout, stderr)
	}
}

func printOutput(out <-chan string, writer io.Writer) {
	for line := range out {
		fmt.Fprintln(writer, line)
	}
}
