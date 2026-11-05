package model

import "strings"

// Config holds runtime configuration for the command tracer.
type Config struct {
	// Filter is the set of command base names to capture.
	// An empty set means all commands are captured.
	Filter map[string]struct{}
}

// ParseFilter creates a Config from a comma-separated list of command names
// (e.g. "go,git,docker"). An empty string means no filtering (capture all).
func ParseFilter(csv string) Config {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return Config{}
	}

	filter := make(map[string]struct{})
	for _, name := range strings.Split(csv, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			filter[name] = struct{}{}
		}
	}
	return Config{Filter: filter}
}

// Match reports whether the given command name passes the filter.
// If no filter is set, all commands match.
func (c Config) Match(name string) bool {
	if len(c.Filter) == 0 {
		return true
	}
	_, ok := c.Filter[name]
	return ok
}
