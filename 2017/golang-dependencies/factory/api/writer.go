package api

import (
	"app/service"
)

type Writer struct {
}

func (r *Writer) Run(services *service.Services) {
	redis := services.GetRedis("default")
	redis.Add(1)
	redis.Add(1)
	redis.Add(2)
	redis.Add(3)
	redis.Add(5)
	redis.Add(8)
	redis.Add(13)
	redis.Add(21)
}
