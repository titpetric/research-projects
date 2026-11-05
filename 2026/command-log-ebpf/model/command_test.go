package model_test

import (
	"commandtrx/model"
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	tests := []struct {
		name     string
		event    model.ExecEvent
		wantName string
		wantSubs []string
		wantStr  string
	}{
		{
			name: "go test with flags",
			event: model.ExecEvent{
				PID: 1, Comm: "go", Filename: "/usr/local/go/bin/go",
				Args:      []string{"test", "-v", "./..."},
				Timestamp: time.Now(),
			},
			wantName: "go",
			wantSubs: []string{"test"},
			wantStr:  "go test",
		},
		{
			name: "docker compose with flags",
			event: model.ExecEvent{
				PID: 2, Comm: "docker", Filename: "/usr/bin/docker",
				Args:      []string{"compose", "-f", "docker-compose.yml", "up"},
				Timestamp: time.Now(),
			},
			wantName: "docker",
			wantSubs: []string{"compose"},
			wantStr:  "docker compose",
		},
		{
			name: "git commit no positional args before flag",
			event: model.ExecEvent{
				PID: 3, Comm: "git", Filename: "/usr/bin/git",
				Args:      []string{"commit", "-m", "initial"},
				Timestamp: time.Now(),
			},
			wantName: "git",
			wantSubs: []string{"commit"},
			wantStr:  "git commit",
		},
		{
			name: "command with no args",
			event: model.ExecEvent{
				PID: 4, Comm: "ls", Filename: "/usr/bin/ls",
				Timestamp: time.Now(),
			},
			wantName: "ls",
			wantSubs: nil,
			wantStr:  "ls",
		},
		{
			name: "only flags",
			event: model.ExecEvent{
				PID: 5, Comm: "go", Filename: "/usr/local/go/bin/go",
				Args:      []string{"-version"},
				Timestamp: time.Now(),
			},
			wantName: "go",
			wantSubs: nil,
			wantStr:  "go",
		},
		{
			name: "empty filename falls back to comm",
			event: model.ExecEvent{
				PID: 6, Comm: "make", Filename: "",
				Args:      []string{"build"},
				Timestamp: time.Now(),
			},
			wantName: "make",
			wantSubs: []string{"build"},
			wantStr:  "make build",
		},
		{
			name: "non-word args are excluded",
			event: model.ExecEvent{
				PID: 7, Comm: "tr", Filename: "/usr/bin/tr",
				Args:      []string{`\n`, `:"`},
				Timestamp: time.Now(),
			},
			wantName: "tr",
			wantSubs: nil,
			wantStr:  "tr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.Summarize(tt.event)
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if len(got.Subcommands) != len(tt.wantSubs) {
				t.Fatalf("Subcommands = %v, want %v", got.Subcommands, tt.wantSubs)
			}
			for i := range tt.wantSubs {
				if got.Subcommands[i] != tt.wantSubs[i] {
					t.Errorf("Subcommands[%d] = %q, want %q", i, got.Subcommands[i], tt.wantSubs[i])
				}
			}
			if s := got.String(); s != tt.wantStr {
				t.Errorf("String() = %q, want %q", s, tt.wantStr)
			}
		})
	}
}

func TestURIHostExtraction(t *testing.T) {
	tests := []struct {
		name    string
		comm    string
		args    []string
		wantStr string
	}{
		{
			name:    "curl with https URL",
			comm:    "curl",
			args:    []string{"-s", "https://api.github.com/repos"},
			wantStr: "curl (api.github.com)",
		},
		{
			name:    "wget with http URL",
			comm:    "wget",
			args:    []string{"http://example.com/file.tar.gz"},
			wantStr: "wget (example.com)",
		},
		{
			name:    "git clone with git:// URL",
			comm:    "git",
			args:    []string{"clone", "git://github.com/user/repo.git"},
			wantStr: "git clone (github.com)",
		},
		{
			name:    "git clone with https URL",
			comm:    "git",
			args:    []string{"clone", "https://github.com/user/repo.git"},
			wantStr: "git clone (github.com)",
		},
		{
			name:    "ssh URI",
			comm:    "ssh",
			args:    []string{"ssh://user@host.example.com/path"},
			wantStr: "ssh (host.example.com)",
		},
		{
			name:    "no URI in args",
			comm:    "go",
			args:    []string{"build", "./..."},
			wantStr: "go build",
		},
		{
			name:    "ftp URL",
			comm:    "curl",
			args:    []string{"ftp://files.example.com/pub/data"},
			wantStr: "curl (files.example.com)",
		},
		{
			name:    "URL with port",
			comm:    "curl",
			args:    []string{"http://localhost:8080/health"},
			wantStr: "curl (localhost)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := model.ExecEvent{
				PID: 1, Comm: tt.comm, Filename: "/usr/bin/" + tt.comm,
				Args: tt.args,
			}
			got := e.Summary().String()
			if got != tt.wantStr {
				t.Errorf("String() = %q, want %q", got, tt.wantStr)
			}
		})
	}
}

func TestExecEventSummary(t *testing.T) {
	e := model.ExecEvent{
		PID: 10, Comm: "go", Filename: "/usr/local/go/bin/go",
		Args:      []string{"build", "-o", "bin/app"},
		Timestamp: time.Now(),
	}
	got := e.Summary()
	if got.Name != "go" || len(got.Subcommands) != 1 || got.Subcommands[0] != "build" {
		t.Errorf("Summary() = %v, want go build", got)
	}
}
