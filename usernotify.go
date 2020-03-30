package main

import (
	"sort"
	"sync"
)

type discordUserMention string
type nickname string

// NotifyMap maps one player name to a discord user.
type NotifyMap struct {
	sync.Mutex
	m map[nickname]map[discordUserMention]bool
}

func newNotifyMap() *NotifyMap {
	n := NotifyMap{}
	n.m = make(map[nickname]map[discordUserMention]bool, 32)
	return &n
}

// Add adds a new nickname to be tracked by Discord User ID
func (n *NotifyMap) Add(discordMention, nick string) {
	n.Lock()
	defer n.Unlock()

	playername := nickname(nick)
	discordName := discordUserMention(discordMention)

	// initialize
	if n.m[playername] == nil {
		n.m[playername] = make(map[discordUserMention]bool, 2)
	}

	// add dc user to tracked users of player
	n.m[playername][discordName] = true
}

// Remove removes a moderator's tracking of a specific user.
func (n *NotifyMap) Remove(discordMention string) {
	n.Lock()
	defer n.Unlock()

	dcMention := discordUserMention(discordMention)

	for playername, dcUserMap := range n.m {
		if len(dcUserMap) == 1 {

			_, ok := dcUserMap[dcMention]
			if ok {
				// player is tracked by dc user
				delete(n.m, playername)
			}

		} else if len(dcUserMap) > 1 {

			_, ok := dcUserMap[dcMention]
			if ok {
				// player is tracked by dc user
				delete(dcUserMap, dcMention)
			}
		}
	}

}

// Tracked checks if a nickname is tracked and returns eithernil or the
// pointer to the requesting user.
func (n *NotifyMap) Tracked(nick string) (mentions []string) {
	n.Lock()
	defer n.Unlock()

	name := nickname(nick)

	dcUsers, ok := n.m[name]

	if !ok {
		return
	}

	mentions = make([]string, 0, len(dcUsers))

	for dcUser := range dcUsers {
		mentions = append(mentions, string(dcUser))
	}

	sort.Sort(byName(mentions))

	return
}
