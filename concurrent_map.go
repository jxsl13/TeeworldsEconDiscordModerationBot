package main

import (
	"sync"
)

type discordChannel string

type channelAddressMap struct {
	mu sync.Mutex
	m  map[discordChannel]address
}

func newChannelAddressMap() channelAddressMap {
	return channelAddressMap{m: make(map[discordChannel]address)}
}

func (a *channelAddressMap) Set(channelID discordChannel, addr address) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.m[channelID] = addr
}

func (a *channelAddressMap) Get(channelID discordChannel) (address, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	addr, ok := a.m[channelID]
	return addr, ok
}

func (a *channelAddressMap) AlreadyRegistered(addr address) (found bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, addr := range a.m {
		if addr == addr {
			found = true
			return
		}
	}
	return
}
