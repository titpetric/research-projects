package api

import (
	"app/service"
)

type Writer struct {
	Redis *service.Redis `inject`
}

func (r *Writer) Run() {
	r.Redis.Add(1)
	r.Redis.Add(1)
	r.Redis.Add(2)
	r.Redis.Add(3)
	r.Redis.Add(5)
	r.Redis.Add(8)
	r.Redis.Add(13)
	r.Redis.Add(21)
}
