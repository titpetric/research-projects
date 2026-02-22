package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Input item structure
type Item struct {
	Command string   `json:"command"`
	Parents []string `json:"parents"`
}

// Internal aggregation struct
type linkAgg struct {
	Source string
	Target string
	Value  int
}

func nodeID(command string, depth int) string {
	return fmt.Sprintf("%s|%d", command, depth)
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	var items []Item
	if err := json.Unmarshal(data, &items); err != nil {
		panic(err)
	}

	linkMap := map[string]*linkAgg{}

	for _, it := range items {
		depth := len(it.Parents)
		if depth == 0 {
			continue
		}

		target := nodeID(it.Command, depth)
		source := nodeID(it.Parents[depth-1], depth-1)

		key := source + "->" + target

		if l, ok := linkMap[key]; ok {
			l.Value++
		} else {
			linkMap[key] = &linkAgg{
				Source: source,
				Target: target,
				Value:  1,
			}
		}
	}

	// Convert to [][]any row format
	rows := make([][]any, 0, len(linkMap))
	for _, l := range linkMap {
		rows = append(rows, []any{l.Source, l.Target, l.Value})
	}

	enc := json.NewEncoder(os.Stdout)
	//enc.SetIndent("", "  ")
	if err := enc.Encode(rows); err != nil {
		panic(err)
	}
}