package api

import (
	"app/service"
	"fmt"
)

type Processor struct {
}

type ProcessorServices interface {
	GetDatabase(string) *service.Database;
	GetRedis(string) *service.Redis;
}

func (r *Processor) Run(services ProcessorServices) string {
	redis := services.GetRedis("default")
	db := services.GetDatabase("default")
	values := redis.Get()
	for _, value := range *values {
		db.Add(value)
	}
	return fmt.Sprintf("%#v", db)
}
