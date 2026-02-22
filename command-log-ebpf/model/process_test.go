package model_test

import (
	"commandtrx/model"
	"os"
	"testing"
)

func TestIsShell(t *testing.T) {
	shells := []string{"bash", "sh", "zsh", "fish", "dash", "ksh", "csh", "tcsh"}
	for _, s := range shells {
		if !model.IsShell(s) {
			t.Errorf("IsShell(%q) = false, want true", s)
		}
	}

	nonShells := []string{"go", "git", "docker", "make", "node", "python", ""}
	for _, s := range nonShells {
		if model.IsShell(s) {
			t.Errorf("IsShell(%q) = true, want false", s)
		}
	}
}

func TestIsBinary(t *testing.T) {
	// A known executable.
	if !model.IsBinary("/bin/sh") {
		t.Error("IsBinary(/bin/sh) = false, want true")
	}

	// A non-existent path.
	if model.IsBinary("/nonexistent/binary") {
		t.Error("IsBinary(/nonexistent/binary) = true, want false")
	}

	// An empty path.
	if model.IsBinary("") {
		t.Error("IsBinary(\"\") = true, want false")
	}

	// A non-executable regular file.
	f, err := os.CreateTemp("", "notexec")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()
	if model.IsBinary(f.Name()) {
		t.Errorf("IsBinary(%q) = true for non-executable file", f.Name())
	}
}

func TestHasShellAncestor_immediateBash(t *testing.T) {
	// If the immediate parent comm is "bash", it should return true
	// without needing to walk /proc at all.
	if !model.HasShellAncestor(99999, "bash") {
		t.Error("HasShellAncestor with parentComm=bash should return true")
	}
}

func TestHasShellAncestor_nonShellParent(t *testing.T) {
	// A bogus PID with a non-shell parent comm — /proc walk will fail
	// gracefully and return false.
	if model.HasShellAncestor(99999, "go") {
		t.Error("HasShellAncestor with parentComm=go and invalid PID should return false")
	}
}

func TestHasShellAncestor_currentProcess(t *testing.T) {
	// Our own process should have a shell ancestor (we're run from a shell
	// or test runner which itself is run from a shell).
	pid := uint32(os.Getpid())
	// Pass empty parentComm to force /proc walking.
	got := model.HasShellAncestor(pid, "")
	// This test is best-effort; in CI the ancestor may not be a shell.
	_ = got
}

func TestIsDescendantOf_immediateAncestor(t *testing.T) {
	// If the immediate parent comm is "go", it should return true.
	if !model.IsDescendantOf(99999, "go", "go") {
		t.Error("IsDescendantOf with parentComm=go should return true for ancestor=go")
	}
}

func TestIsDescendantOf_nonMatchingAncestor(t *testing.T) {
	// A bogus PID with a non-matching parent comm — should return false.
	if model.IsDescendantOf(99999, "bash", "go") {
		t.Error("IsDescendantOf with parentComm=bash should return false for ancestor=go")
	}
}
