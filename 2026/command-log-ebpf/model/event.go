package model

import "time"

// TaskCommLen is the maximum length of the command name from the kernel (TASK_COMM_LEN).
const TaskCommLen = 16

// ArgMax is the maximum length of a single argument captured from execve.
const ArgMax = 256

// ArgsMax is the maximum number of arguments captured from execve.
const ArgsMax = 20

// ExecEvent represents a raw exec event captured from the kernel via eBPF
// when a process calls execve.
type ExecEvent struct {
	// PID is the process ID of the executed command.
	PID uint32

	// PPID is the parent process ID.
	PPID uint32

	// UID is the user ID of the process owner.
	UID uint32

	// Comm is the short command name (up to TASK_COMM_LEN bytes from kernel).
	Comm string

	// ParentComm is the immediate parent's command name.
	ParentComm string

	// Filename is the path to the executable being invoked.
	Filename string

	// Args holds the command-line arguments passed to the executable.
	Args []string

	// Timestamp is when the exec event was captured.
	Timestamp time.Time
}

// Summary returns a CommandSummary derived from this event.
func (e ExecEvent) Summary() CommandSummary {
	return Summarize(e)
}
