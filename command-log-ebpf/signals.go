package main

import (
	"os"
	"syscall"
)

func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
