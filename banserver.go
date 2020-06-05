package main

import (
	"errors"
	"fmt"
	"sort"
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

// BanServer handles the ban parsing
type BanServer struct {
	mu      sync.Mutex
	BanList []Ban
}

// GetIDFrom IP looks for the banlist index based on IP
func (b *BanServer) GetIDFrom(ip string) (id int, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for idx, currBan := range b.BanList {
		if currBan.Player.IP == ip {
			return idx, true
		}
	}

	return 0, false
}

// Ban a player for a specific time and reason.
func (b *BanServer) Ban(p Player, duration time.Duration, reason string) {
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
			sort.Sort(byBantime(b.BanList))
			return
		}
	}

	// add new ban
	b.BanList = append(b.BanList, Ban{
		Player:    p,
		ExpiresAt: time.Now().Add(duration),
		Reason:    reason,
	})
	sort.Sort(byBantime(b.BanList))
}

// GetBan by ID
func (b *BanServer) GetBan(id int) (foundBan Ban, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if id < 0 || len(b.BanList) <= id {
		return Ban{}, ErrInvalidIndex
	}

	return b.BanList[id], nil
}

// GetBanByNameAndReason returns a ban that has a specific reason and a specific player name.
func (b *BanServer) GetBanByNameAndReason(name, reason string) (bb Ban, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ban := range b.BanList {
		if ban.Player.Name == name && ban.Reason == reason {
			return ban, true
		}
	}

	return Ban{}, false
}

// SetPlayerAfterwards associates a new player object with a ban if the IPs of both match.
func (b *BanServer) SetPlayerAfterwards(p Player) (ok bool) {
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

// UnbanIP removes a ban.
func (b *BanServer) UnbanIP(ip string) (Ban, error) {
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

// UnbanIndex removes a ban by its index.
func (b *BanServer) UnbanIndex(index int) (Ban, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if index < 0 || len(b.BanList)-1 < index {
		return Ban{}, ErrInvalidIndex
	}

	ban := b.BanList[index]
	b.BanList = append(b.BanList[:index], b.BanList[index+1:]...)
	return ban, nil
}

// UnbanAll empties the banlist.
func (b *BanServer) UnbanAll() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.BanList = b.BanList[:0]
}

// Bans returns a list of all bans.
func (b *BanServer) Bans() []Ban {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]Ban, len(b.BanList))
	copy(result, b.BanList)
	return result
}

// Size returns the number of current bans.
func (b *BanServer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.BanList)
}

// String formats the banlist properly.
func (b *BanServer) String() string {
	bans := b.Bans()

	sb := strings.Builder{}

	for idx, ban := range bans {
		sb.WriteString(fmt.Sprintf("idx=%-2d %9s '%s' (%s)\n", idx, ban.ExpiresAt.Sub(time.Now()).Round(time.Second), ban.Player.Name, ban.Reason))
	}

	return sb.String()

}

func newBanServer() BanServer {
	return BanServer{BanList: make([]Ban, 0, 8)}
}
