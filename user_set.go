package main

import "sync"

type userSet struct {
	mu sync.Mutex
	m  map[string]bool
}

func newUserSet() userSet {
	return userSet{m: make(map[string]bool)}
}

func (u *userSet) Contains(s string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	val, ok := u.m[s]
	if !ok {
		return false
	}

	return val
}

func (u *userSet) Add(s string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.m[s] = true
}

func (u *userSet) Remove(s string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.m, s)
}

func (u *userSet) Reset() {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.m = make(map[string]bool)
}
