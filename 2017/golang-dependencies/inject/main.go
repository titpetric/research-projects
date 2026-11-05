package main

import (
	"fmt"

	"app/api"
	"app/service"

	"github.com/stephanos/inject"
)

func main() {
	injector := inject.New()
	injector.Map(&service.Redis{Name: "default"})
	injector.Map(func() *service.Database {
		return &service.Database{Name: "default"}
	})

	writer := api.Writer{}
	processor := api.Processor{}

	injector.Apply(&writer)
	injector.Apply(&processor)

	writer.Run()
	result := processor.Run()

	fmt.Println(result)
}
