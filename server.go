package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
// vanilla
[2019-10-13 03:15:53][server]: player is ready. ClientID=0 addr=127.0.0.1:61292
[2019-10-13 03:15:53][server]: 'qwertzuio    ' -> 'qwertzuio'
[2019-10-13 03:15:53][server]: player has entered the game. ClientID=0 addr=127.0.0.1:61292

[2019-10-13 03:16:06][server]: client dropped. cid=0 addr=127.0.0.1:61292 reason=''

// ddrace
[2020-03-19 12:43:14][server]: player is ready. ClientID=0 addr=<{123.0.0.1:51151}>
*/

var (
	// ErrPlayerNotFound is returned by the ip matching player search function if no player was found.
	ErrPlayerNotFound = errors.New("player not found")

	// 0: full 1: ID 2: IP
	playerReadyRegex = regexp.MustCompile(`\[server\]: player is ready. ClientID=([\d]+) addr=[^\da-fA-F]*([\d:\.a-fA-F]+)[^\da-fA-F]*:[\d]+[^\da-fA-F]*$`)

	// 0 full 1: trimmed nickname
	playerGetNameRegex = regexp.MustCompile(`\[server\]: '.{0,20}' -> '(.{0,20})'$`)

	// 0: full 1: ID 2: IP
	playerEnteredRegex = regexp.MustCompile(`\[server\]: player has entered the game. ClientID=([\d]+) addr=[^\da-fA-F]*([\d:\.a-fA-F]+)[^\da-fA-F]*:[\d]+[^\da-fA-F]*$`)

	// 0: full 1: ID 2: IP 3: reason
	playerLeftRegex = regexp.MustCompile(`\[server\]: client dropped. cid=([\d]+) addr=[^\da-fA-F]*([\d:\.a-fA-F]+)[^\da-fA-F]*:[\d]+[^\da-fA-F]* reason='(.*)'$`)

	banAddRegex   = regexp.MustCompile(`\[net_ban\]: banned '(.*)' for ([\d]+) minute[s]? \((.*)\)$`)
	banAddIPRegex = regexp.MustCompile(`\[net_ban\]: '(.*)' banned for ([\d]+) minute[s]? \((.*)\)$`)

	banRemoveIndexRegex = regexp.MustCompile(`\[net_ban\]: unbanned index [\d]+ \('(.+)'\)`)
	banRemoveIPRegex    = regexp.MustCompile(`\[net_ban\]: unbanned '(.+)'`)
	banExpiredRegex     = regexp.MustCompile(`\[net_ban\]: ban '(.+)' expired$`)
)

const (
	// These states discribe a player's current state
	stateEmpty         = 0
	stateReadyNameless = 1
	stateNamed         = 2
	stateIngame        = 3
)

type player struct {
	Name    string
	Clan    string
	ID      int
	Address address
	State   int
}

func (p *player) Valid() bool {
	return p.ID >= 0
}

func (p *player) Online() bool {
	return p.State == stateIngame
}

func (p *player) Clear() {
	id := p.ID
	*p = player{}
	p.ID = id //ID stays the same
}

type server struct {
	sync.RWMutex    // guards slots object
	players         [64]player
	lastReadyPlayer int // the last mentioned ready player gets a new nickname
	BanServer       banServer
	JoinCallbacks   []playerCallback
	LeaveCallbacks  []playerCallback
}

type playerCallback func(player)

func newServer() *server {
	srv := &server{
		BanServer:      newBanServer(),
		JoinCallbacks:  make([]playerCallback, 0, 1),
		LeaveCallbacks: make([]playerCallback, 0, 1),
	}

	for idx := range srv.players {
		srv.Lock()
		srv.players[idx].ID = idx
		srv.Unlock()
	}

	return srv
}

// ParseLine parses a line from econ or logs, which affects the internal server state.
func (s *server) ParseLine(line string, notify *NotifyMap) (consumed bool, logline string) {
	if strings.Contains(line, "[server]") {

		match := []string{}

		// this has priority over any other parsed info, as the order might become incorrect
		// if this is parsed for example after the playerReadyRegex
		match = playerGetNameRegex.FindStringSubmatch(line)
		if len(match) == 2 {
			name := match[1]
			s.Lock()
			s.players[s.lastReadyPlayer].Name = name
			s.players[s.lastReadyPlayer].State = stateNamed
			s.Unlock()
			return true, ""
		}

		// ready client, no name, yet
		match = playerReadyRegex.FindStringSubmatch(line)
		if len(match) == 3 {
			id, _ := strconv.Atoi(match[1])
			ip := match[2]
			s.lastReadyPlayer = id

			s.Lock()
			s.players[id].Address = address(ip)
			s.players[id].State = stateReadyNameless
			s.Unlock()

			return true, ""
		}

		// player actually reaches ingame state
		match = playerEnteredRegex.FindStringSubmatch(line)
		if len(match) == 3 {
			id, _ := strconv.Atoi(match[1])

			s.Lock()
			s.players[id].State = stateIngame
			player := s.players[id]
			s.Unlock()

			s.handleJoin(player)

			// notification requested
			if notify != nil {
				var sb strings.Builder

				mentions := notify.Tracked(player.Name)
				if len(mentions) > 0 {

					for idx, mention := range mentions {
						sb.WriteString(mention)
						if idx < len(mentions)-1 {
							sb.WriteString(" ")
						}
					}

					return true, fmt.Sprintf("[server]: '%s' joined the server with id %d\n%s", player.Name, id, sb.String())
				}

			}

			if config.LogLevel >= 2 {
				return true, fmt.Sprintf("[server]: '%s' joined the server with id %d", player.Name, id)
			}

			return true, ""
		}

		// player leaves
		match = playerLeftRegex.FindStringSubmatch(line)
		if len(match) == 4 {
			id, _ := strconv.Atoi(match[1])

			s.Lock()
			// make copy
			player := s.players[id]

			// clear player slot
			s.players[id].Clear()
			s.Unlock()

			s.handleLeave(player)

			if config.LogLevel >= 2 {
				return true, fmt.Sprintf("[server]: '%s' left the server, id was %d", player.Name, id)
			}
		}
	} else if strings.Contains(line, "[net_ban]") {
		matches := []string{}

		matches = banAddRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 3) {
			ip := address(matches[1])
			minutes, _ := strconv.Atoi(matches[2])
			reason := matches[3]

			// returns (unknown) dummy if player was not found
			p := s.PlayerByIP(ip)
			duration := time.Minute * time.Duration(minutes)

			s.BanServer.Ban(p, duration, reason)

			// player found, send nickname
			return true, fmt.Sprintf("**[bans]**: '%s' banned for %9s with reason: '%s'", p.Name, duration.Round(time.Second), reason)
		}

		matches = banAddIPRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 3) {
			ip := address(matches[1])
			minutes, _ := strconv.Atoi(matches[2])
			reason := matches[3]

			p := s.PlayerByIP(ip)
			duration := time.Minute * time.Duration(minutes)

			s.BanServer.Ban(p, duration, reason)

			// player found, send nickname
			return true, fmt.Sprintf("**[bans]**: '%s' banned for %9s with reason: '%s'", p.Name, duration.Round(time.Second), reason)
		}

		matches = banExpiredRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 1) {
			ip := address(matches[1])

			ban, err := s.BanServer.UnbanIP(ip)

			if err != nil {
				return true, fmt.Sprintf("[bans]: ban of '%s' expired", ban.Player.Name)
			}

			return true, fmt.Sprintf("[bans]: ban of '%s' expired (%s)", ban.Player.Name, ban.Reason)
		}

		matches = banRemoveIndexRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 1) {
			ip := address(matches[1])

			ban, err := s.BanServer.UnbanIP(ip)

			if err != nil {
				return true, fmt.Sprintf("[bans]: unbanned '%s'", ban.Player.Name)
			}

			return true, fmt.Sprintf("[bans]: unbanned '%s' (%s)", ban.Player.Name, ban.Reason)
		}

		matches = banRemoveIPRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 1) {
			ip := address(matches[1])

			ban, err := s.BanServer.UnbanIP(ip)

			if err != nil {
				return true, fmt.Sprintf("[bans]: unbanned '%s'", ban.Player.Name)
			}
			return true, fmt.Sprintf("[bans]: unbanned '%s' (%s)", ban.Player.Name, ban.Reason)
		}

	}

	return false, ""
}

func (s *server) Player(id int) player {
	if id < 0 || 63 < id {
		return player{
			Name:    "(unknown)",
			Clan:    "",
			Address: "",
			ID:      -1,
		}
	}

	s.Lock()
	defer s.Unlock()

	return s.players[id]
}

// PlayerByIP returns a dummy player with a negative ID if no player with expected IP was found.
func (s *server) PlayerByIP(ip address) player {
	s.Lock()
	defer s.Unlock()

	for _, p := range s.players {
		if p.Online() && p.Address == ip {
			return p
		}
	}

	return player{
		Name:    "(unknown)",
		Clan:    "",
		Address: ip,
		ID:      -1,
	}
}

func (s *server) Status() []player {
	playerList := make([]player, 0, 32)

	s.RLock()
	defer s.RUnlock()

	for _, p := range s.players {
		if p.Online() {
			playerList = append(playerList, p)
		}
	}

	return playerList
}

func (s *server) AddJoinHandler(handler playerCallback) {
	s.JoinCallbacks = append(s.JoinCallbacks, handler)
}

func (s *server) AddLeaveHandler(handler playerCallback) {
	s.LeaveCallbacks = append(s.LeaveCallbacks, handler)
}

func (s *server) handleJoin(p player) {
	for _, cb := range s.JoinCallbacks {
		go cb(p)
	}
}

func (s *server) handleLeave(p player) {
	for _, cb := range s.LeaveCallbacks {
		go cb(p)
	}
}
