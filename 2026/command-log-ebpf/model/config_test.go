package model_test

import (
	"commandtrx/model"
	"testing"
)

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected filter size, 0 means capture-all
	}{
		{"empty string captures all", "", 0},
		{"whitespace captures all", "  ", 0},
		{"single command", "go", 1},
		{"multiple commands", "go,git,docker", 3},
		{"with spaces", " go , git , docker ", 3},
		{"trailing comma", "go,git,", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := model.ParseFilter(tt.input)
			if len(cfg.Filter) != tt.want {
				t.Errorf("len(Filter) = %d, want %d", len(cfg.Filter), tt.want)
			}
		})
	}
}

func TestConfigMatch(t *testing.T) {
	tests := []struct {
		name   string
		csv    string
		cmd    string
		expect bool
	}{
		{"no filter matches all", "", "anything", true},
		{"filter includes command", "go,git", "go", true},
		{"filter excludes command", "go,git", "docker", false},
		{"filter single match", "docker", "docker", true},
		{"filter single no match", "docker", "go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := model.ParseFilter(tt.csv)
			if got := cfg.Match(tt.cmd); got != tt.expect {
				t.Errorf("Match(%q) = %v, want %v", tt.cmd, got, tt.expect)
			}
		})
	}
}
