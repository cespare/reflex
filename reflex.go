package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// A Reflex is a single watch + command to execute.
type Reflex struct {
	id           int
	source       string // Describes what config/line defines this Reflex
	startService bool
	backlog      Backlog
	matcher      Matcher
	onlyFiles    bool
	onlyDirs     bool
	command      []string
	subSymbol    string
	done         chan struct{}

	mu      *sync.Mutex // protects killed and running
	killed  bool
	running bool
	timeout time.Duration

	// Used for services (startService = true)
	cmd *exec.Cmd
	tty *os.File
}

// NewReflex prepares a Reflex from a Config, with sanity checking.
func NewReflex(c *Config) (*Reflex, error) {
	matcher, err := ParseMatchers(c.regexes, c.inverseRegexes, c.globs, c.inverseGlobs)
	if err != nil {
		return nil, fmt.Errorf("error parsing glob/regex: %s", err)
	}
	if !c.allFiles {
		matcher = multiMatcher{defaultExcludeMatcher, matcher}
	}
	if len(c.command) == 0 {
		return nil, errors.New("must give command to execute")
	}

	if c.subSymbol == "" {
		return nil, errors.New("substitution symbol must be non-empty")
	}

	substitution := false
	for _, part := range c.command {
		if strings.Contains(part, c.subSymbol) {
			substitution = true
			break
		}
	}

	var backlog Backlog
	if substitution {
		if c.startService {
			return nil, errors.New("using --start-service does not work with a command that has a substitution symbol")
		}
		backlog = NewUniqueFilesBacklog()
	} else {
		backlog = NewUnifiedBacklog()
	}

	if c.onlyFiles && c.onlyDirs {
		return nil, errors.New("cannot specify both --only-files and --only-dirs")
	}

	if c.shutdownTimeout <= 0 {
		return nil, errors.New("shutdown timeout cannot be <= 0")
	}

	reflex := &Reflex{
		id:           reflexID,
		source:       c.source,
		startService: c.startService,
		backlog:      backlog,
		matcher:      matcher,
		onlyFiles:    c.onlyFiles,
		onlyDirs:     c.onlyDirs,
		command:      c.command,
		subSymbol:    c.subSymbol,
		done:         make(chan struct{}),
		timeout:      c.shutdownTimeout,
		mu:           &sync.Mutex{},
	}
	reflexID++

	return reflex, nil
}

func (r *Reflex) String() string {
	var buf bytes.Buffer
	fmt.Fprintln(&buf, "Reflex from", r.source)
	fmt.Fprintln(&buf, "| ID:", r.id)
	for _, matcherInfo := range strings.Split(r.matcher.String(), "\n") {
		fmt.Fprintln(&buf, "|", matcherInfo)
	}
	if r.onlyFiles {
		fmt.Fprintln(&buf, "| Only matching files.")
	} else if r.onlyDirs {
		fmt.Fprintln(&buf, "| Only matching directories.")
	}
	if !r.startService {
		fmt.Fprintln(&buf, "| Substitution symbol", r.subSymbol)
	}
	replacer := strings.NewReplacer(r.subSymbol, "<filename>")
	command := make([]string, len(r.command))
	for i, part := range r.command {
		command[i] = replacer.Replace(part)
	}
	fmt.Fprintln(&buf, "| Command:", command)
	fmt.Fprintln(&buf, "+---------")
	return buf.String()
}

// filterMatching passes on messages matching the regex/glob.
func (r *Reflex) filterMatching(out chan<- string, in <-chan string) {
	for name := range in {
		if !r.matcher.Match(name) {
			continue
		}

		if r.onlyFiles || r.onlyDirs {
			stat, err := os.Stat(name)
			if err != nil {
				continue
			}
			if (r.onlyFiles && stat.IsDir()) || (r.onlyDirs && !stat.IsDir()) {
				continue
			}
		}
		out <- name
	}
}

// batch receives file notification events and batches them up. It's a bit
// tricky, but here's what it accomplishes:
// * When we initially get a message, wait a bit and batch messages before
//   trying to send anything. This is because the file events come in bursts.
// * Once it's time to send, don't do it until the out channel is unblocked.
//   In the meantime, keep batching. When we've sent off all the batched
//   messages, go back to the beginning.
func (r *Reflex) batch(out chan<- string, in <-chan string) {

	const silenceInterval = 300 * time.Millisecond

	for name := range in {
		r.backlog.Add(name)
		timer := time.NewTimer(silenceInterval)
	outer:
		for {
			select {
			case name := <-in:
				r.backlog.Add(name)
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(silenceInterval)
			case <-timer.C:
				for {
					select {
					case name := <-in:
						r.backlog.Add(name)
					case out <- r.backlog.Next():
						if r.backlog.RemoveOne() {
							break outer
						}
					}
				}
			}
		}
	}
}

// runEach runs the command on each name that comes through the names channel.
// Each {} is replaced by the name of the file. The output of the command is
// passed line-by-line to the stdout chan.
func (r *Reflex) runEach(names <-chan string) {
	for name := range names {
		if r.startService {
			if r.Running() {
				infoPrintln(r.id, "Killing service")
				r.terminate()
			}
			infoPrintln(r.id, "Starting service")
			r.runCommand(name, stdout)
		} else {
			r.runCommand(name, stdout)
			<-r.done
			r.mu.Lock()
			r.running = false
			r.mu.Unlock()
		}
	}
}

func (r *Reflex) terminate() {
	r.mu.Lock()
	r.killed = true
	r.mu.Unlock()
	// Write ascii 3 (what you get from ^C) to the controlling pty.
	// (This won't do anything if the process already died as the write will
	// simply fail.)
	r.tty.Write([]byte{3})

	timer := time.NewTimer(r.timeout)
	sig := syscall.SIGINT
	for {
		select {
		case <-r.done:
			return
		case <-timer.C:
			if sig == syscall.SIGINT {
				infoPrintln(r.id, "Sending SIGINT signal...")
			} else {
				infoPrintln(r.id, "Sending SIGKILL signal...")
			}

			// Instead of killing the process, we want to kill its
			// whole pgroup in order to clean up any children the
			// process may have created.
			if err := syscall.Kill(-1*r.cmd.Process.Pid, sig); err != nil {
				infoPrintln(r.id, "Error killing:", err)
				if err.(syscall.Errno) == syscall.ESRCH { // no such process
					return
				}
			}
			// After SIGINT doesn't do anything, try SIGKILL next.
			timer.Reset(r.timeout)
			sig = syscall.SIGKILL
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

var seqCommands = &sync.Mutex{}

// runCommand runs the given Command. All output is passed line-by-line to the
// stdout channel.
func (r *Reflex) runCommand(name string, stdout chan<- OutMsg) {
	command := replaceSubSymbol(r.command, r.subSymbol, name)
	cmd := exec.Command(command[0], command[1:]...)
	r.cmd = cmd

	if flagSequential {
		seqCommands.Lock()
	}

	tty, err := pty.Start(cmd)
	if err != nil {
		infoPrintln(r.id, err)
		return
	}
	r.tty = tty

	// Handle pty size.
	chResize := make(chan os.Signal, 1)
	signal.Notify(chResize, syscall.SIGWINCH)
	go func() {
		for range chResize {
			// Intentionally ignore errors in case stdout is not a tty
			pty.InheritSize(os.Stdout, tty)
		}
	}()
	chResize <- syscall.SIGWINCH // Initial resize.

	go func() {
		scanner := bufio.NewScanner(tty)
		for scanner.Scan() {
			stdout <- OutMsg{r.id, scanner.Text()}
		}
		// Intentionally ignoring scanner.Err() for now. Unfortunately,
		// the pty returns a read error when the child dies naturally,
		// so I'm just going to ignore errors here unless I can find a
		// better way to handle it.
	}()

	r.mu.Lock()
	r.running = true
	r.mu.Unlock()
	go func() {
		err := cmd.Wait()
		if !r.Killed() && err != nil {
			stdout <- OutMsg{r.id, fmt.Sprintf("(error exit: %s)", err)}
		}
		r.done <- struct{}{}

		signal.Stop(chResize)
		close(chResize)

		if flagSequential {
			seqCommands.Unlock()
		}
	}()
}

func (r *Reflex) Start(changes <-chan string) {
	filtered := make(chan string)
	batched := make(chan string)
	go r.filterMatching(filtered, changes)
	go r.batch(batched, filtered)
	go r.runEach(batched)
	if r.startService {
		// Easy hack to kick off the initial start.
		infoPrintln(r.id, "Starting service")
		r.runCommand("", stdout)
	}
}

func (r *Reflex) Killed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.killed
}

func (r *Reflex) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}
