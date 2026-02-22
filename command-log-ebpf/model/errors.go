package model

import "errors"

var (
	// ErrBPFLoad is returned when the eBPF program fails to load.
	ErrBPFLoad = errors.New("failed to load eBPF program")

	// ErrAttachProbe is returned when attaching to a kernel probe fails.
	ErrAttachProbe = errors.New("failed to attach probe")

	// ErrRingBuf is returned when reading from the ring buffer fails.
	ErrRingBuf = errors.New("failed to read ring buffer")

	// ErrInvalidEvent is returned when an event cannot be parsed.
	ErrInvalidEvent = errors.New("invalid event data")
)
