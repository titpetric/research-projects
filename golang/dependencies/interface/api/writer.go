package api

import (
	"app/service"
)

type Writer struct {
}

type WriterServices interface {
	GetRedis(string) *service.Redis;
}

func (r *Writer) Run(services WriterServices) {
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
