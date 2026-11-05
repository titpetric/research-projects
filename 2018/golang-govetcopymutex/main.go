package main

import (
	"fmt"
	"sync"
)

type S struct {
	sync.Mutex
}

func (S) First() *S {
	return &S{sync.Mutex{}}
}

func (_ S) Second() *S {
	return &S{sync.Mutex{}}
}

func main() {
	s1 := S{}.First()
	s2 := S{}.Second()
	fmt.Println("mutexes", s1, s2)
}
