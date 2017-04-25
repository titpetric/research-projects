package service

type Database struct {
	Name       string
	Count, Sum int
}

func (r *Database) Add(value int) {
	r.Count++
	r.Sum += value
}
