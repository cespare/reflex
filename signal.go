package main

import (
	"fmt"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func SignalFromString(rawSignal string) (syscall.Signal, error) {
	if !strings.HasPrefix(rawSignal, "SIG") {
		return 0, fmt.Errorf("signal has to start with SIG prefix. Got: %s", rawSignal)
	}

	return unix.SignalNum(rawSignal), nil
}

func SignalToString(sig syscall.Signal) string {
	return unix.SignalName(sig)
}
