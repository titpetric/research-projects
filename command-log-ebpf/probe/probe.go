package probe

import (
	"bytes"
	"commandtrx/model"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/cilium/ebpf/ringbuf"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang-20 -type event bpf execve.c -- -I headers

// Run loads the eBPF program, attaches to the execve tracepoint, and streams
// matching events to stdout. It blocks until ctx is cancelled.
func Run(ctx context.Context, cfg model.Config) error {
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("%w: %v", model.ErrBPFLoad, err)
	}

	objs := bpfObjects{}
	if err := loadBpfObjects(&objs, nil); err != nil {
		return fmt.Errorf("%w: %v", model.ErrBPFLoad, err)
	}
	defer objs.Close()

	tp, err := link.Tracepoint("syscalls", "sys_enter_execve", objs.TracepointSysEnterExecve, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", model.ErrAttachProbe, err)
	}
	defer tp.Close()

	rd, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		return fmt.Errorf("%w: %v", model.ErrRingBuf, err)
	}
	defer rd.Close()

	go func() {
		<-ctx.Done()
		rd.Close()
	}()

	log.Println("Tracing execve calls... Press Ctrl+C to stop.")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		record, err := rd.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil
			}
			return fmt.Errorf("%w: %v", model.ErrRingBuf, err)
		}

		evt, err := parseEvent(record.RawSample)
		if err != nil {
			continue
		}

		if !model.IsBinary(evt.Filename) {
			continue
		}

		if !model.HasShellAncestor(evt.PID, evt.ParentComm) {
			continue
		}

		summary := evt.Summary()
		
		// Skip if command itself is a known shell
		if model.IsShell(summary.Name) {
			continue
		}

		// Skip ignored commands (test binaries, sleep, etc.)
		if model.IsIgnored(summary.Name) {
			continue
		}

		// Skip if 'go' is in the ancestor chain (descendants of 'go')
		if model.IsDescendantOf(evt.PID, evt.ParentComm, "go") {
			continue
		}

		if !cfg.Match(summary.Name) {
			continue
		}

		cmd := summary.Name
		if len(summary.Subcommands) > 0 {
			cmd += " " + strings.Join(summary.Subcommands, " ")
		}
		ancestors := model.AncestorChain(evt.PID, evt.ParentComm)
		slices.Reverse(ancestors)
		ancestorsJSON, _ := json.Marshal(ancestors)
		fmt.Printf("- pid: %d\n  command: %s\n  binary: %s\n  parents: %s\n",
			evt.PID, cmd, evt.Filename, string(ancestorsJSON))
		if summary.Host != "" {
			fmt.Printf("  host: %s\n", summary.Host)
		}
	}
}

// parseEvent converts the raw ring buffer bytes into a model.ExecEvent.
func parseEvent(raw []byte) (model.ExecEvent, error) {
	var be bpfEvent
	if len(raw) < int(bpfEventSize()) {
		return model.ExecEvent{}, model.ErrInvalidEvent
	}

	be = *(*bpfEvent)(bytesPtr(raw))

	comm := cStr(be.Comm[:])
	parentComm := cStr(be.ParentComm[:])
	filename := cStr(be.Filename[:])
	args := parseArgs(be.Args[:], be.ArgsSize)

	return model.ExecEvent{
		PID:        be.Pid,
		PPID:       be.Ppid,
		UID:        be.Uid,
		Comm:       comm,
		ParentComm: parentComm,
		Filename:   filename,
		Args:       args,
		Timestamp:  time.Now(),
	}, nil
}

// parseArgs splits a null-separated byte buffer into individual argument strings.
func parseArgs(buf []byte, size uint32) []string {
	if size == 0 {
		return nil
	}
	if int(size) > len(buf) {
		size = uint32(len(buf))
	}
	trimmed := buf[:size]
	// Remove trailing null bytes.
	trimmed = bytes.TrimRight(trimmed, "\x00")
	if len(trimmed) == 0 {
		return nil
	}

	parts := bytes.Split(trimmed, []byte{0})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) > 0 {
			out = append(out, string(p))
		}
	}
	return out
}

// cStr extracts a Go string from a null-terminated C byte slice.
func cStr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
