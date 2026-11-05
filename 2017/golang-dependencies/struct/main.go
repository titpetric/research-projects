package main

import (
	"fmt"

	"app/api"
	"app/service"
)

func main() {
	redis := &service.Redis{Name: "default"}
	writer := api.Writer{
		Redis: redis,
	}
	processor := api.Processor{
		Redis: redis,
		DB:    &service.Database{Name: "default"},
	}

	writer.Run()
	result := processor.Run()

	fmt.Println(result)
}
