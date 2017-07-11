package main

import "sync"

type Defer struct {
	sync.Mutex
	value int
}

//go:noinline
func (r *Defer) defer1() {
	r.Lock()
	defer r.Unlock()
	r.value ++
}

//go:noinline
func (r *Defer) defer2() {
	r.Lock()
	r.value ++
	r.Unlock()
}
