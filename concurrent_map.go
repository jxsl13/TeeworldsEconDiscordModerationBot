package main

import (
	"sort"
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

func (a *channelAddressMap) GetAddresses() []address {
	a.mu.Lock()
	result := make([]address, 0, len(a.m))

	for _, addr := range a.m {
		result = append(result, addr)
	}
	a.mu.Unlock()

	sort.Sort(byAddress(result))
	return result
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

func (a *channelAddressMap) RemoveAddress(addr address) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for key, value := range a.m {
		if value == addr {
			delete(a.m, key)
			break
		}
	}
	return
}

func (a *channelAddressMap) RemoveChannel(chann discordChannel) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.m, chann)
	return
}

func (a *channelAddressMap) AlreadyRegistered(addr address) (found bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, serverAddr := range a.m {
		if serverAddr == addr {
			found = true
			return
		}
	}
	return
}
