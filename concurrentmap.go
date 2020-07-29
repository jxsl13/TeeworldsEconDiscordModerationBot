package main

import (
	"sort"
	"sync"
)

type discordChannel string

// ChannelAddressMap maps a discord channel to a server address
type ChannelAddressMap struct {
	mu sync.Mutex
	m  map[discordChannel]Address
}

func newChannelAddressMap() ChannelAddressMap {
	return ChannelAddressMap{m: make(map[discordChannel]Address)}
}

// GetAddresses returns a sorted list of all actively mapped addresses
func (a *ChannelAddressMap) GetAddresses() []Address {
	a.mu.Lock()
	result := make([]Address, 0, len(a.m))

	for _, addr := range a.m {
		result = append(result, addr)
	}
	a.mu.Unlock()

	sort.Sort(byAddress(result))
	return result
}

// Set connects a discord channel ID with a server Address
func (a *ChannelAddressMap) Set(channelID discordChannel, addr Address) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.m[channelID] = addr
}

// Get returns the Address that is associated with the used channel.
func (a *ChannelAddressMap) Get(channelID discordChannel) (Address, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	addr, ok := a.m[channelID]
	return addr, ok
}

// RemoveAddress removes server address from mapping
func (a *ChannelAddressMap) RemoveAddress(addr Address) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for key, value := range a.m {
		if value == addr {
			delete(a.m, key)
			break
		}
	}
}

// RemoveChannel removes channel address from mapping
func (a *ChannelAddressMap) RemoveChannel(chann discordChannel) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.m, chann)
}

// AlreadyRegistered checks if a server address is already registered to a discord channel.
func (a *ChannelAddressMap) AlreadyRegistered(addr Address) (found bool) {
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
