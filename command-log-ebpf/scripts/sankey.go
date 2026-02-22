package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ExecutionEntry struct {
	PID     int      `yaml:"pid"`
	Command string   `yaml:"command"`
	Binary  string   `yaml:"binary"`
	Parents []string `yaml:"parents"`
}

type Flow struct {
	Source string
	Target string
	Count  int
}

// Grouping defines pattern-based command grouping
// Key: glob pattern, Value: group name
var Grouping = map[string]string{
	"*.test": "go test binary",
}

func main() {
	// Read execution log
	data, err := os.ReadFile("execution.log.yml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	var entries []ExecutionEntry
	err = yaml.Unmarshal(data, &entries)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing YAML: %v\n", err)
		os.Exit(1)
	}

	// Build flows
	flowMap := make(map[string]int)
	for _, entry := range entries {
		// Skip entries that have a *.test parent
		if hasTestParent(entry.Parents) {
			continue
		}

		cmd := groupCommand(entry.Command)
		depth := len(entry.Parents)
		groupedParents := make([]string, len(entry.Parents))
		for i, p := range entry.Parents {
			groupedParents[i] = groupCommand(p)
		}

		if len(groupedParents) == 0 {
			// Root command
			key := fmt.Sprintf("START -> %s [depth 0]", cmd)
			flowMap[key]++
		} else {
			// Connect last parent to current command
			parent := groupedParents[len(groupedParents)-1]
			key := fmt.Sprintf("%s -> %s [depth %d]", parent, cmd, depth)
			flowMap[key]++

			// Add intermediate flows for chains
			for i := 0; i < len(groupedParents)-1; i++ {
				key := fmt.Sprintf("%s -> %s [depth %d]", groupedParents[i], groupedParents[i+1], i+1)
				flowMap[key]++
			}
		}
	}

	// Sort flows by count descending
	var flows []Flow
	for flowStr, count := range flowMap {
		parts := strings.Split(flowStr, " -> ")
		flows = append(flows, Flow{
			Source: parts[0],
			Target: parts[1],
			Count:  count,
		})
	}

	sort.Slice(flows, func(i, j int) bool {
		return flows[i].Count > flows[j].Count
	})

	// Generate PlantUML
	puml := strings.Builder{}
	puml.WriteString("@startuml\n")
	puml.WriteString("title Command Execution Flow\n\n")
	puml.WriteString("skinparam componentStyle rectangle\n")
	puml.WriteString("skinparam defaultTextAlignment center\n")
	puml.WriteString("top to bottom direction\n\n")

	// Define components with colors based on count
	seenNodes := make(map[string]bool)
	for _, f := range flows {
		src := sanitize(f.Source)
		tgt := sanitize(f.Target)

		if !seenNodes[src] {
			color := getColor(f.Count)
			puml.WriteString(fmt.Sprintf("component \"%s\" as %s %s\n", f.Source, src, color))
			seenNodes[src] = true
		}
		if !seenNodes[tgt] {
			color := getColor(f.Count)
			// Include depth in target display
			display := f.Target
			puml.WriteString(fmt.Sprintf("component \"%s\" as %s %s\n", display, tgt, color))
			seenNodes[tgt] = true
		}
	}

	puml.WriteString("\n' Relationships\n")
	for _, f := range flows {
		src := sanitize(f.Source)
		tgt := sanitize(f.Target)
		puml.WriteString(fmt.Sprintf("%s --> %s : %d\n", src, tgt, f.Count))
	}

	puml.WriteString("\n@enduml\n")

	// Write to file
	outFile := "execution-sankey.puml"
	err = os.WriteFile(outFile, []byte(puml.String()), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s\n", outFile)
	fmt.Printf("Total flows: %d\n", len(flows))
}

func sanitize(s string) string {
	// Replace spaces and special chars with underscores for PlantUML
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "[", "_")
	s = strings.ReplaceAll(s, "]", "_")
	return s
}

func getColor(count int) string {
	// Color intensity based on flow count
	switch {
	case count > 900:
		return "#FF4444" // Red for high
	case count > 250:
		return "#FFB347" // Orange
	case count > 50:
		return "#FFEB99" // Yellow
	case count > 10:
		return "#C2DFFF" // Light blue
	default:
		return "#E8E8E8" // Light gray
	}
}

func groupCommand(cmd string) string {
	// Apply grouping rules to command name
	for pattern, groupName := range Grouping {
		if matched, _ := filepath.Match(pattern, cmd); matched {
			return groupName
		}
	}
	return cmd
}

func hasTestParent(parents []string) bool {
	// Check if any parent matches *.test pattern
	for _, p := range parents {
		if matched, _ := filepath.Match("*.test", p); matched {
			return true
		}
	}
	return false
}
