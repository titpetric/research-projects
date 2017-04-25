package api

import (
	"app/service"
	"fmt"
)

type Processor struct {
	DB    *service.Database `inject`
	Redis *service.Redis    `inject`
}

func (r *Processor) Run() string {
	values := r.Redis.Get()
	for _, value := range *values {
		r.DB.Add(value)
	}
	return fmt.Sprintf("%#v", r.DB)
}
