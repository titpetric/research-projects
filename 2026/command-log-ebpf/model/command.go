package model

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// validArg matches arguments that look like real command words: lowercase
// letters, digits, dots, slashes, and underscores. Rejects escape sequences,
// punctuation-only values, and other non-command noise.
var validArg = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_./-]*$`)

// CommandSummary is the processed representation of a command execution,
// containing only the command name and subcommands up to the first flag argument.
type CommandSummary struct {
	// Name is the base name of the executable (e.g. "go", "docker", "git").
	Name string

	// Subcommands holds positional arguments before the first flag.
	// For example, "go test ./..." yields Subcommands: ["test", "./..."].
	// For "docker compose -f foo.yml up", it yields Subcommands: ["compose"].
	Subcommands []string

	// Host is the hostname extracted from the first URI found in any argument.
	// Empty if no URI was detected.
	Host string

	// Event is the original event from which this summary was derived.
	Event ExecEvent
}

// String returns a human-readable representation such as "go test ./...".
// If a URI was detected in the arguments, the hostname is appended.
func (c CommandSummary) String() string {
	parts := []string{c.Name}
	parts = append(parts, c.Subcommands...)
	if c.Host != "" {
		parts = append(parts, "("+c.Host+")")
	}
	return strings.Join(parts, " ")
}

// uriSchemes are the schemes we recognise when scanning arguments for URIs.
var uriSchemes = []string{"http://", "https://", "ftp://", "ftps://", "ssh://", "git://"}

// findHost scans all arguments for the first value that contains a URI and
// returns its hostname. It returns "" if none is found.
func findHost(args []string) string {
	for _, arg := range args {
		for _, scheme := range uriSchemes {
			if strings.Contains(arg, scheme) {
				u, err := url.Parse(arg)
				if err == nil && u.Hostname() != "" {
					return u.Hostname()
				}
			}
		}
	}
	return ""
}

// hasURIScheme reports whether s contains a recognised URI scheme.
func hasURIScheme(s string) bool {
	for _, scheme := range uriSchemes {
		if strings.Contains(s, scheme) {
			return true
		}
	}
	return false
}

// Summarize extracts a CommandSummary from an ExecEvent by taking the base
// executable name and all arguments up until the first flag (an argument
// starting with "-").
func Summarize(e ExecEvent) CommandSummary {
	name := filepath.Base(e.Filename)
	if name == "" || name == "." {
		name = e.Comm
	}

	var subs []string
	for _, arg := range e.Args {
		if strings.HasPrefix(arg, "-") {
			break
		}
		if hasURIScheme(arg) {
			continue
		}
		if !validArg.MatchString(arg) {
			break
		}
		subs = append(subs, arg)
	}

	return CommandSummary{
		Name:        name,
		Subcommands: subs,
		Host:        findHost(e.Args),
		Event:       e,
	}
}
