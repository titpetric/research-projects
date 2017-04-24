package api

import (
	"app/service"
	"fmt"
)

type Processor struct {
}

func (r *Processor) Run(services *service.Services) string {
	redis := services.GetRedis("default")
	db := services.GetDatabase("default")
	values := redis.Get()
	for _, value := range *values {
		db.Add(value)
	}
	return fmt.Sprintf("%#v", db)
}
