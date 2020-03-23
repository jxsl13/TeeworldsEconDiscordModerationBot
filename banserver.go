package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNoCorrespondingBanFound is returned if the passed ip could not be unbanned.
	ErrNoCorrespondingBanFound = errors.New("could not find a corresponding match to unban")

	// ErrInvalidIndex is returned if an invalid index is being passed.
	ErrInvalidIndex = errors.New("invalid index")
)

type ban struct {
	Player    player
	ExpiresAt time.Time
	Reason    string
}

func (b *ban) Expired() bool {
	return time.Now().After(b.ExpiresAt)
}

type banServer struct {
	mu      sync.Mutex
	BanList []ban
}

func (b *banServer) Ban(p player, duration time.Duration, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// overwrite existing ban
	for idx, currBan := range b.BanList {
		if currBan.Player.Address == p.Address {
			b.BanList[idx] = ban{
				Player:    p,
				ExpiresAt: time.Now().Add(duration),
				Reason:    reason,
			}
			return
		}
	}

	// add new ban
	b.BanList = append(b.BanList, ban{
		Player:    p,
		ExpiresAt: time.Now().Add(duration),
		Reason:    reason,
	})
}

func (b *banServer) GetBanByNameAndReason(name, reason string) (bb ban, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ban := range b.BanList {
		if ban.Player.Name == name && ban.Reason == reason {
			return ban, true
		}
	}

	return ban{}, false
}

func (b *banServer) SetPlayerAfterwards(p player) (ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for idx, ban := range b.BanList {
		if ban.Player.Address == p.Address {
			b.BanList[idx].Player = p
			return true
		}
	}
	return false
}

func (b *banServer) UnbanIP(ip address) (ban, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	position := -1
	for idx, ban := range b.BanList {
		if ban.Player.Address == ip {
			position = idx
			break
		}
	}

	if position >= 0 {
		ban := b.BanList[position]
		b.BanList = append(b.BanList[:position], b.BanList[position+1:]...)
		return ban, nil
	}

	return ban{}, ErrNoCorrespondingBanFound
}

func (b *banServer) UnbanIndex(index int) (ban, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if index < 0 || len(b.BanList)-1 < index {
		return ban{}, ErrInvalidIndex
	}

	ban := b.BanList[index]
	b.BanList = append(b.BanList[:index], b.BanList[index+1:]...)
	return ban, nil
}

func (b *banServer) Bans() []ban {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]ban, len(b.BanList))
	copy(result, b.BanList)
	return result
}

func (b *banServer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.BanList)
}

func (b *banServer) String() string {
	bans := b.Bans()

	sb := strings.Builder{}

	for idx, ban := range bans {
		sb.WriteString(fmt.Sprintf("idx=%-2d %9s '%s' (%s)\n", idx, ban.ExpiresAt.Sub(time.Now()).Round(time.Second), ban.Player.Name, ban.Reason))
	}

	return sb.String()

}

func newBanServer() banServer {
	return banServer{BanList: make([]ban, 0, 8)}
}
