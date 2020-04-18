package main

import (
	"sort"
	"sync"
)

type commandSet struct {
	mu sync.Mutex
	m  map[string]bool
}

func newCommandSet() commandSet {
	return commandSet{m: make(map[string]bool)}
}

func (u *commandSet) Commands() (commands []string) {
	u.mu.Lock()

	commands = make([]string, 0, len(u.m))

	for command := range u.m {
		commands = append(commands, command)
	}
	u.mu.Unlock()

	sort.Sort(byName(commands))

	return
}

func (u *commandSet) Contains(s string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	val, ok := u.m[s]
	if !ok {
		return false
	}

	return val
}

func (u *commandSet) Add(s string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.m[s] = true
}

func (u *commandSet) Remove(s string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.m, s)
}

func (u *commandSet) Reset() {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.m = make(map[string]bool)
}
