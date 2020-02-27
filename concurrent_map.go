package main

import (
	"sync"
)

type channelAddressMap struct {
	mu sync.Mutex
	m  map[string]string
}

func newChannelAddressMap() channelAddressMap {

	return channelAddressMap{m: make(map[string]string)}
}

func (a *channelAddressMap) Set(channelID, address string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.m[channelID] = address
}

func (a *channelAddressMap) Get(channelID string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	addr, ok := a.m[channelID]
	return addr, ok
}

func (a *channelAddressMap) AlreadyRegistered(address string) (found bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, addr := range a.m {
		if addr == address {
			found = true
			return
		}
	}
	return
}
