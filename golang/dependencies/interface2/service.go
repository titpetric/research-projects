package main

import "app/service"

type serviceFactory struct {
	// may be extended with singletons
	redis *service.Redis
}

// Returns singleton redis instance
func (d *serviceFactory) GetRedis(name string) *service.Redis {
	if d.redis == nil {
		d.redis = &service.Redis{Name: name}
	}
	return d.redis
}

func (d *serviceFactory) GetDatabase(name string) *service.Database {
	return &service.Database{Name: name}
}
