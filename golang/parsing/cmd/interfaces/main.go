package main

import (
	"fmt"
	"os"

	"go/ast"
	"go/parser"
	"go/token"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./interfaces [file.go, ...]")
		os.Exit(255)
	}

	for _, filename := range os.Args[1:] {
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
		if err != nil {
			fmt.Println("Error parsing file: " + filename)
			os.Exit(255)
		}

		// traverse all tokens
		ast.Inspect(node, func(n ast.Node) bool {
			switch t := n.(type) {
			// find variable declarations
			case *ast.TypeSpec:
				// which are public
				if t.Name.IsExported() {
					switch t.Type.(type) {
					// and are interfaces
					case *ast.InterfaceType:
						fmt.Println(t.Name.Name)
					}
				}
			}
			return true
		})
	}
}
