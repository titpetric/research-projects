package service

type Services struct {
	// may be extended with singletons
	redis *Redis
}

// Returns singleton redis instance
func (d *Services) GetRedis(name string) *Redis {
	if d.redis == nil {
		d.redis = &Redis{Name: name}
	}
	return d.redis
}

func (d *Services) GetDatabase(name string) *Database {
	return &Database{Name: name}
}
