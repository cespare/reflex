package main

import (
	"fmt"
	"io"
	"strings"
)

type Decoration int

const (
	DecorationNone = iota
	DecorationPlain
	DecorationFancy
)

const (
	colorRed = 31
	// ANSI colors -- using 32 - 36
	colorStart = 32
	numColors  = 5
)

type OutMsg struct {
	reflexID int
	msg      string
}

func infoPrintln(id int, args ...interface{}) {
	stdout <- OutMsg{id, strings.TrimSpace(fmt.Sprintln(args...))}
}
func infoPrintf(id int, format string, args ...interface{}) {
	stdout <- OutMsg{id, fmt.Sprintf(format, args...)}
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
			color = colorRed
		}
		fmt.Fprintf(writer, "\x1b[01;%dm%s ", color, tag)
	} else if decoration == DecorationPlain {
		fmt.Fprintf(writer, tag+" ")
	}
	fmt.Fprint(writer, msg.msg)
	if decoration == DecorationFancy {
		fmt.Fprintf(writer, "\x1b[m")
	}
	if !strings.HasSuffix(msg.msg, "\n") {
		fmt.Fprintln(writer)
	}
}

func printOutput(out <-chan OutMsg, outWriter io.Writer) {
	for msg := range out {
		printMsg(msg, outWriter)
	}
}
