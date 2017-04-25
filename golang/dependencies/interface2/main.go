package main

import (
	"fmt"

	"app/api"
)

func main() {
	// would pass all config here
	services := &serviceFactory{}

	writer := &api.Writer{}
	writer.Run(services)

	processor := api.Processor{}
	result := processor.Run(services)

	fmt.Println(result)
}
