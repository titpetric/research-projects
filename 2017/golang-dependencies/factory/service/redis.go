package service

type Redis struct {
	Name string
	List []int
}

func (r *Redis) Add(value int) {
	r.List = append(r.List, value)
}

func (r *Redis) Get() *[]int {
	return &r.List
}
