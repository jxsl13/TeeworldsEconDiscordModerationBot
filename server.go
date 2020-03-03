package main

import (
	"log"
	"sync"
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
	mu    sync.RWMutex // guards server object
	slots [64]playerSlot
}

func (s *server) join(id int, p player) {
	if id < 0 || id >= 64 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.slots[id].Player = p
	s.slots[id].Occupied = true
}

func (s *server) leave(id int) {
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
