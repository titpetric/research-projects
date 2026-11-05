package main

import (
	"fmt"

	"app/api"
	"app/service"
)

func main() {
	// would pass all config here
	services := &service.Services{}

	writer := &api.Writer{}
	writer.Run(services)

	processor := api.Processor{}
	result := processor.Run(services)

	fmt.Println(result)
}
