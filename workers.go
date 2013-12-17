package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/howeyc/fsnotify"
	"github.com/kr/pty"
)

var seqCommands = &sync.Mutex{}

type OutMsg struct {
	reflexID int
	message  string
}

const (
	// ANSI colors -- using 32 - 36
	colorStart = 32
	numColors  = 5
)

func infoPrintln(id int, args ...interface{}) {
	stdout <- OutMsg{id, strings.TrimSpace(fmt.Sprintln(args...))}
}
func infoPrintf(id int, format string, args ...interface{}) {
	stdout <- OutMsg{id, fmt.Sprintf(format, args...)}
}

func walker(watcher *fsnotify.Watcher) filepath.WalkFunc {
	return func(path string, f os.FileInfo, err error) error {
		if err != nil || !f.IsDir() {
			// TODO: Is there some other thing we should be doing to handle errors? When watching large
			// directories that have lots of programs modifying them (esp. if they're making tempfiles along the
			// way), we often get errors.
			return nil
		}
		if err := watcher.Watch(path); err != nil {
			// TODO: handle this somehow?
			infoPrintf(-1, "Error while watching new path %s: %s\n", path, err)
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
	if err := filepath.Walk(root, walker(watcher)); err != nil {
		// TODO: handle this somehow?
		infoPrintf(-1, "Error while walking path %s: %s\n", root, err)
	}

	for {
		select {
		case e := <-watcher.Event:
			path := strings.TrimPrefix(e.Name, "./")
			names <- path
			if e.IsCreate() {
				if err := filepath.Walk(path, walker(watcher)); err != nil {
					// TODO: handle this somehow?
					infoPrintf(-1, "Error while walking path %s: %s\n", path, err)
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

// filterMatching passes on messages matching the regex/glob.
func filterMatching(in <-chan string, out chan<- string, reflex *Reflex) {
	for name := range in {
		if reflex.useRegex {
			if !reflex.regex.MatchString(name) {
				continue
			}
		} else {
			matches, err := filepath.Match(reflex.glob, name)
			// TODO: It would be good to notify the user on an error here.
			if err != nil {
				infoPrintln(reflex.id, "Error matching glob:", err)
				continue
			}
			if !matches {
				continue
			}
		}
		// TODO: These only match if the file/dir still exists...not sure if there's a better way.
		if reflex.onlyFiles || reflex.onlyDirs {
			stat, err := os.Stat(name)
			if err != nil {
				continue
			}
			if (reflex.onlyFiles && stat.IsDir()) || (reflex.onlyDirs && !stat.IsDir()) {
				continue
			}
		}
		out <- name
	}
}

// batch receives realtime file notification events and batches them up. It's a bit tricky, but here's what
// it accomplishes:
// * When we initially get a message, wait a bit and batch messages before trying to send anything. This is
//	 because the file events come in quick bursts.
// * Once it's time to send, don't do it until the out channel is unblocked. In the meantime, keep batching.
//   When we've sent off all the batched messages, go back to the beginning.
func batch(in <-chan string, out chan<- string, reflex *Reflex) {
	for name := range in {
		reflex.backlog.Add(name)
		timer := time.NewTimer(300 * time.Millisecond)
	outer:
		for {
			select {
			case name := <-in:
				reflex.backlog.Add(name)
			case <-timer.C:
				for {
					select {
					case name := <-in:
						reflex.backlog.Add(name)
					case out <- reflex.backlog.Next():
						if reflex.backlog.RemoveOne() {
							break outer
						}
					}
				}
			}
		}
	}
}

// runEach runs the command on each name that comes through the names channel. Each {} is replaced by the name
// of the file. The output of the command is passed line-by-line to the stdout chan.
func runEach(names <-chan string, reflex *Reflex) {
	for name := range names {
		if reflex.startService {
			if reflex.done != nil {
				infoPrintln(reflex.id, "Killing service")
				terminate(reflex)
			}
			infoPrintln(reflex.id, "Starting service")
			runCommand(reflex, name, stdout)
		} else {
			runCommand(reflex, name, stdout)
			<-reflex.done
			reflex.done = nil
		}
	}
}

func terminate(reflex *Reflex) {
	first := true
	sig := syscall.SIGINT
	reflex.mut.Lock()
	reflex.killed = true
	reflex.mut.Unlock()
	// Write ascii 3 (what you get from ^C) to the controlling pty.
	reflex.tty.Write([]byte{3})

	timer := time.NewTimer(500 * time.Millisecond)
	for {
		select {
		case <-reflex.done:
			return
		case <-timer.C:
			if first {
				infoPrintln(reflex.id, "Sending SIGINT signal...")
			} else {
				infoPrintln(reflex.id, "Error killing process. Trying again with SIGKILL...")
			}

			if err := reflex.cmd.Process.Signal(sig); err != nil {
				infoPrintln(reflex.id, "Error killing:", err)
				// TODO: is there a better way to detect this?
				if err.Error() == "no such process" {
					return
				}
			}
			if first {
				first = false
				sig = syscall.SIGKILL
			}
		}
	}
}

func replaceSubSymbol(command []string, subSymbol string, name string) []string {
	replacer := strings.NewReplacer(subSymbol, name)
	newCommand := make([]string, len(command))
	for i, c := range command {
		newCommand[i] = replacer.Replace(c)
	}
	return newCommand
}

// runCommand runs the given Command. All output is passed line-by-line to the stdout channel.
func runCommand(reflex *Reflex, name string, stdout chan<- OutMsg) {
	command := replaceSubSymbol(reflex.command, reflex.subSymbol, name)
	cmd := exec.Command(command[0], command[1:]...)
	reflex.cmd = cmd

	if flagSequential {
		seqCommands.Lock()
	}

	tty, err := pty.Start(cmd)
	if err != nil {
		infoPrintln(reflex.id, err)
		return
	}
	reflex.tty = tty

	go func() {
		scanner := bufio.NewScanner(tty)
		for scanner.Scan() {
			stdout <- OutMsg{reflex.id, scanner.Text()}
		}
		// Intentionally ignoring scanner.Err() for now.
		// Unfortunately, the pty returns a read error when the child dies naturally, so I'm just going to ignore
		// errors here unless I can find a better way to handle it.
	}()

	done := make(chan struct{})
	reflex.done = done
	go func() {
		err := cmd.Wait()
		reflex.mut.Lock()
		killed := reflex.killed
		reflex.mut.Unlock()
		if !killed && err != nil {
			stdout <- OutMsg{reflex.id, fmt.Sprintf("(error exit: %s)", err)}
		}
		done <- struct{}{}
		if flagSequential {
			seqCommands.Unlock()
		}
	}()
}

func printMsg(msg OutMsg, writer io.Writer) {
	tag := ""
	if decoration == DecorationFancy || decoration == DecorationPlain {
		if msg.reflexID < 0 {
			tag = "[info]"
		} else {
			tag = fmt.Sprintf("[%02d]", msg.reflexID)
		}
	}

	if decoration == DecorationFancy {
		color := (msg.reflexID % numColors) + colorStart
		if reflexID < 0 {
			color = 31 // red
		}
		fmt.Fprintf(writer, "\x1b[01;%dm%s ", color, tag)
	} else if decoration == DecorationPlain {
		fmt.Fprintf(writer, tag+" ")
	}
	fmt.Fprint(writer, msg.message)
	if decoration == DecorationFancy {
		fmt.Fprintf(writer, "\x1b[m")
	}
	fmt.Fprintln(writer)
}

func printOutput(out <-chan OutMsg, outWriter io.Writer) {
	for msg := range out {
		printMsg(msg, outWriter)
	}
}
