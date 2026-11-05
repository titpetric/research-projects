package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// shells is the set of known shell command names.
var shells = map[string]struct{}{
	"bash": {}, "sh": {}, "zsh": {}, "fish": {},
	"dash": {}, "ksh": {}, "csh": {}, "tcsh": {},
	"ash": {}, "mksh": {}, "elvish": {}, "nu": {},
	"xonsh": {}, "ion": {},
}

// IsShell reports whether name is a known shell.
func IsShell(name string) bool {
	_, ok := shells[name]
	return ok
}

// ignored is the set of command names to skip from tracing output.
var ignored = map[string]struct{}{
	"sleep": {},
}

// IsIgnored reports whether a command should be skipped from output.
// It excludes test binaries (*.test) and known uninteresting commands.
func IsIgnored(name string) bool {
	// Skip test binaries
	if strings.HasSuffix(name, ".test") {
		return true
	}
	_, ok := ignored[name]
	return ok
}

// noise is the set of process names excluded from the displayed ancestor chain.
var noise = map[string]struct{}{
	"bash": {}, "sh": {}, "dash": {}, "zsh": {}, "fish": {},
	"ksh": {}, "csh": {}, "tcsh": {}, "ash": {}, "mksh": {},
	"elvish": {}, "nu": {}, "xonsh": {}, "ion": {},
	"sudo": {},
}

func isNoise(name string) bool {
	_, ok := noise[name]
	return ok
}

// maxAncestorDepth limits how far up the process tree we walk.
const maxAncestorDepth = 16

// AncestorChain walks the process tree from pid upward and returns the list of
// ancestor command names, closest parent first, stopping at (and including)
// the outermost shell ancestor. parentComm is used for the immediate parent to
// avoid an extra /proc read.
// Example result: ["bun", "bash", "mc", "bash"]
// AncestorChain returns the ancestor breadcrumb for display, with shells and
// sudo filtered out.
func AncestorChain(pid uint32, parentComm string) []string {
	chain := ancestorChainRaw(pid, parentComm)
	filtered := make([]string, 0, len(chain))
	for _, name := range chain {
		if !isNoise(name) {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

// HasShellAncestor checks whether the process with the given pid has a shell
// anywhere in its ancestor chain.
func HasShellAncestor(pid uint32, parentComm string) bool {
	for _, name := range ancestorChainRaw(pid, parentComm) {
		if IsShell(name) {
			return true
		}
	}
	return false
}

// IsDescendantOf checks whether the process with the given pid has a specific
// ancestor in its ancestor chain.
func IsDescendantOf(pid uint32, parentComm string, ancestor string) bool {
	for _, name := range ancestorChainRaw(pid, parentComm) {
		if name == ancestor {
			return true
		}
	}
	return false
}

// ancestorChainRaw walks the process tree and returns all ancestors up to the
// outermost shell, without filtering.
func ancestorChainRaw(pid uint32, parentComm string) []string {
	var chain []string

	cur := readPPID(pid)
	if cur > 1 {
		// Always re-read the parent's full name to avoid truncation
		first := readComm(cur)
		if first == "" {
			first = parentComm
		}
		chain = append(chain, first)

		cur = readPPID(cur)
		for i := 1; i < maxAncestorDepth && cur > 1; i++ {
			comm := readComm(cur)
			if comm == "" {
				break
			}
			chain = append(chain, comm)
			cur = readPPID(cur)
		}
	} else if parentComm != "" {
		chain = []string{parentComm}
	}

	// Trim to the last (outermost) shell in the chain.
	lastShell := -1
	for i, name := range chain {
		if IsShell(name) {
			lastShell = i
		}
	}
	if lastShell >= 0 {
		chain = chain[:lastShell+1]
	}
	return chain
}

// IsBinary reports whether filename points to an existing executable regular file.
func IsBinary(filename string) bool {
	if filename == "" {
		return false
	}
	fi, err := os.Stat(filename)
	if err != nil {
		return false
	}
	if !fi.Mode().IsRegular() {
		return false
	}
	return fi.Mode().Perm()&0111 != 0
}

// readComm reads the comm (command name) for a given pid from /proc.
// First tries /proc/[pid]/cmdline for the full command, falls back to /proc/[pid]/comm.
func readComm(pid uint32) string {
	// Try cmdline first for the full command name
	pidStr := strconv.FormatUint(uint64(pid), 10)
	data, err := os.ReadFile(filepath.Join("/proc", pidStr, "cmdline"))
	if err == nil && len(data) > 0 {
		// cmdline is null-separated; extract the first element (the binary path)
		endIdx := strings.IndexByte(string(data), '\x00')
		if endIdx < 0 {
			endIdx = len(data)
		}
		binPath := string(data[:endIdx])
		if binPath != "" {
			// Get just the basename from the path
			return filepath.Base(binPath)
		}
	}

	// Fall back to comm (limited to 15 chars but always available)
	data, err = os.ReadFile(filepath.Join("/proc", pidStr, "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// readPPID reads the parent pid for a given pid from /proc.
func readPPID(pid uint32) uint32 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PPid:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "PPid:"))
			n, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return 0
			}
			return uint32(n)
		}
	}
	return 0
}
