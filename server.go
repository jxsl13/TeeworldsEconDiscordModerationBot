package main

import (
	"errors"
	"log"
	"sync"
)

var (
	// ErrNotFound is returned by the ip matching player search function if no player was found.
	ErrNotFound = errors.New("player not found")
)

type player struct {
	Name    string
	Clan    string
	ID      int
	Address address
}

type playerSlot struct {
	Player   player
	Occupied bool
}

type server struct {
	mu    sync.RWMutex // guards slots object
	slots [64]playerSlot

	BanServer banServer
}

func newServer() *server {
	return &server{BanServer: newBanServer()}
}

func (s *server) Join(id int, p player) {
	if id < 0 || id >= 64 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.slots[id].Player = p
	s.slots[id].Occupied = true
}

func (s *server) Leave(id int) {
	if id < 0 || id >= 64 {
		log.Println("Invalid leaving ID")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.slots[id].Player = player{}
	s.slots[id].Occupied = false
}

func (s *server) Player(id int) player {
	if id < 0 || 64 <= id {
		log.Println("Invalid leaving ID")
		return player{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.slots[id].Player
}

func (s *server) PlayerByIP(ip address) (player, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, slot := range s.slots {
		if slot.Occupied && slot.Player.Address == ip {
			return slot.Player, nil
		}
	}

	return player{}, ErrNotFound
}

func (s *server) Status() []player {
	players := make([]player, 0, 32)

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ps := range s.slots {
		if ps.Occupied {
			players = append(players, ps.Player)
		}
	}

	return players
}
