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

// Ban represents a banned player with the ban expiration and the ban reason.
type Ban struct {
	Player    Player
	ExpiresAt time.Time
	Reason    string
}

// Expired tests if the ban is already expired.
func (b *Ban) Expired() bool {
	return time.Now().After(b.ExpiresAt)
}

type banServer struct {
	mu      sync.Mutex
	BanList []Ban
}

func (b *banServer) GetIDFrom(ip string) (id int, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for idx, currBan := range b.BanList {
		if currBan.Player.IP == ip {
			return idx, true
		}
	}

	return 0, false
}

func (b *banServer) Ban(p Player, duration time.Duration, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// overwrite existing ban
	for idx, currBan := range b.BanList {
		if currBan.Player.IP == p.IP {
			b.BanList[idx] = Ban{
				Player:    p,
				ExpiresAt: time.Now().Add(duration),
				Reason:    reason,
			}
			return
		}
	}

	// add new ban
	b.BanList = append(b.BanList, Ban{
		Player:    p,
		ExpiresAt: time.Now().Add(duration),
		Reason:    reason,
	})
}

func (b *banServer) GetBan(id int) (foundBan Ban, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if id < 0 || len(b.BanList) <= id {
		return Ban{}, ErrInvalidIndex
	}

	return b.BanList[id], nil
}

func (b *banServer) GetBanByNameAndReason(name, reason string) (bb Ban, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ban := range b.BanList {
		if ban.Player.Name == name && ban.Reason == reason {
			return ban, true
		}
	}

	return Ban{}, false
}

func (b *banServer) SetPlayerAfterwards(p Player) (ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for idx, ban := range b.BanList {
		if ban.Player.IP == p.IP {
			b.BanList[idx].Player = p
			return true
		}
	}
	return false
}

func (b *banServer) UnbanIP(ip string) (Ban, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	position := -1
	for idx, ban := range b.BanList {
		if ban.Player.IP == ip {
			position = idx
			break
		}
	}

	if position >= 0 {
		ban := b.BanList[position]
		b.BanList = append(b.BanList[:position], b.BanList[position+1:]...)
		return ban, nil
	}

	return Ban{}, ErrNoCorrespondingBanFound
}

func (b *banServer) UnbanIndex(index int) (Ban, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if index < 0 || len(b.BanList)-1 < index {
		return Ban{}, ErrInvalidIndex
	}

	ban := b.BanList[index]
	b.BanList = append(b.BanList[:index], b.BanList[index+1:]...)
	return ban, nil
}

func (b *banServer) UnbanAll() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.BanList = b.BanList[:0]
}

func (b *banServer) Bans() []Ban {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]Ban, len(b.BanList))
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
	return banServer{BanList: make([]Ban, 0, 8)}
}
