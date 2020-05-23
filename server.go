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


 */

var (
	// ErrPlayerNotFound is returned by the ip matching player search function if no player was found.
	ErrPlayerNotFound = errors.New("player not found")

	// 0: full 1: ID 2: IP 3: port 4: version 5: name 6: clan 7: country
	playerEnteredRegex = regexp.MustCompile(`id=([\d]+) addr=([a-fA-F0-9\.\:\[\]]+):([\d]+) version=(\d+) name='(.{0,20})' clan='(.{0,16})' country=([-\d]+)$`)

	// 0: full 1: ID 2: IP 3: reason
	playerLeftRegex = regexp.MustCompile(`id=([\d]+) addr=([a-fA-F0-9\.\:\[\]]+) reason='(.*)'$`)

	// logLevel: net_ban
	banAddRegex   = regexp.MustCompile(`^banned '(.*)' for ([\d]+) minute[s]? \((.*)\)$`)
	banAddIPRegex = regexp.MustCompile(`^'(.*)' banned for ([\d]+) minute[s]? \((.*)\)$`)

	banRemoveIndexRegex = regexp.MustCompile(`^unbanned index [\d]+ \('(.+)'\)`)
	banRemoveIPRegex    = regexp.MustCompile(`^unbanned '(.+)'`)
	banExpiredRegex     = regexp.MustCompile(`^ban '(.+)' expired$`)
	banRemoveAll        = regexp.MustCompile(`^unbanned all entries$`)
)

// Player represents an ingame player.
type Player struct {
	ID      int
	Name    string
	Clan    string
	Country int
	IP      string
	Port    int
	Version int
}

// Valid returns true if the player's ID is valid.
func (p *Player) Valid() bool {
	return p.ID >= 0 && len(p.IP) > 0 && p.Port > 0
}

// Clear resets the player to default values except for its ID
func (p *Player) Clear() {
	id := p.ID
	*p = Player{}
	p.ID = id //ID stays the same
}

// Server represents a tracked Teeworlds server
type Server struct {
	sync.RWMutex   // guards slots object
	players        [64]Player
	BanServer      banServer
	JoinCallbacks  []PlayerCallback
	LeaveCallbacks []PlayerCallback
}

// PlayerCallback is a function that takes a player as parameter.
type PlayerCallback func(Player)

// NewServer creates a new empty server
func NewServer() *Server {
	srv := &Server{
		BanServer:      newBanServer(),
		JoinCallbacks:  make([]PlayerCallback, 0, 1),
		LeaveCallbacks: make([]PlayerCallback, 0, 1),
	}

	for idx := range srv.players {
		srv.Lock()
		srv.players[idx].ID = idx
		srv.Unlock()
	}

	return srv
}

// ParseLine parses a line from econ or logs, which affects the internal server state.
func (s *Server) ParseLine(logLevel, logLine string, notify *NotifyMap) (consumed bool, formatedString string) {

	switch logLevel {
	case "client_enter":
		match := playerEnteredRegex.FindStringSubmatch(logLine)
		if len(match) == 8 {

			id, _ := strconv.Atoi(match[1])
			port, _ := strconv.Atoi(match[3])
			version, _ := strconv.Atoi(match[4])
			country, _ := strconv.Atoi(match[7])

			player := Player{
				ID:      id,
				Name:    match[5],
				Clan:    match[6],
				Country: country,
				IP:      match[2],
				Port:    port,
				Version: version,
			}

			s.Lock()
			s.players[id] = player
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

					return true, fmt.Sprintf("[server]: '%s' joined the server with id %d\n%s", Escape(player.Name), id, sb.String())
				}

			}

			if config.LogLevel >= 2 {
				return true, fmt.Sprintf("[server]: '%s' joined the server with id %d", player.Name, id)
			}

			return true, ""
		}
	case "client_drop":
		// player leaves
		match := playerLeftRegex.FindStringSubmatch(logLine)
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
				return true, fmt.Sprintf("[server]: '%s' left the server, id was %d", Escape(player.Name), id)
			}
			return true, ""
		}
	case "net_ban":
		matches := banAddRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 3) {
			ip := matches[1]
			minutes, _ := strconv.Atoi(matches[2])
			reason := matches[3]

			// returns (unknown) dummy if player was not found
			p := s.PlayerByIP(ip)
			duration := time.Minute * time.Duration(minutes)

			s.BanServer.Ban(p, duration, reason)

			// player found, send nickname
			return true, fmt.Sprintf("**[bans]**: '%s' banned for %9s with reason: '%s'", p.Name, duration.Round(time.Second), reason)
		}

		matches = banAddIPRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 3) {
			ip := matches[1]
			minutes, _ := strconv.Atoi(matches[2])
			reason := matches[3]

			p := s.PlayerByIP(ip)
			duration := time.Minute * time.Duration(minutes)

			s.BanServer.Ban(p, duration, reason)

			// player found, send nickname
			return true, fmt.Sprintf("**[bans]**: '%s' banned for %9s with reason: '%s'", p.Name, duration.Round(time.Second), reason)
		}

		matches = banExpiredRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 1) {
			ip := matches[1]

			ban, err := s.BanServer.UnbanIP(ip)

			if err != nil {
				return true, fmt.Sprintf("[bans]: ban of '%s' expired", ban.Player.Name)
			}

			return true, fmt.Sprintf("[bans]: ban of '%s' expired (%s)", ban.Player.Name, ban.Reason)
		}

		matches = banRemoveIndexRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 1) {
			ip := matches[1]

			ban, err := s.BanServer.UnbanIP(ip)

			if err != nil {
				return true, fmt.Sprintf("[bans]: unbanned '%s'", ban.Player.Name)
			}

			return true, fmt.Sprintf("[bans]: unbanned '%s' (%s)", ban.Player.Name, ban.Reason)
		}

		matches = banRemoveIPRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 1) {
			ip := matches[1]

			ban, err := s.BanServer.UnbanIP(ip)

			if err != nil {
				return true, fmt.Sprintf("[bans]: unbanned '%s'", ban.Player.Name)
			}
			return true, fmt.Sprintf("[bans]: unbanned '%s' (%s)", ban.Player.Name, ban.Reason)
		}

		matches = banRemoveAll.FindStringSubmatch(logLine)
		if len(matches) == 1 {
			s.BanServer.UnbanAll()
			return true, fmt.Sprintf("[bans]: unbanned all players.")
		}
	}

	return false, ""
}

// Player returns the player by its ID.
func (s *Server) Player(id int) Player {
	if id < 0 || 63 < id {
		return Player{
			Name: "(unknown)",
			ID:   -1,
		}
	}

	s.Lock()
	defer s.Unlock()

	return s.players[id]
}

// PlayerByIP returns a dummy player with a negative ID if no player with expected IP was found.
func (s *Server) PlayerByIP(ip string) Player {
	s.Lock()
	defer s.Unlock()

	for _, p := range s.players {
		if p.IP == ip {
			return p
		}
	}

	return Player{
		Name: "(unknown)",
		ID:   -1,
		IP:   ip,
	}
}

// Status returns a list of all online players
func (s *Server) Status() []Player {
	playerList := make([]Player, 0, 32)

	s.RLock()
	defer s.RUnlock()

	for _, p := range s.players {
		if p.Valid() {
			playerList = append(playerList, p)
		}
	}

	return playerList
}

// AddJoinHandler add a new player join handler.
func (s *Server) AddJoinHandler(handler PlayerCallback) {
	s.JoinCallbacks = append(s.JoinCallbacks, handler)
}

// AddLeaveHandler add a new player leaving handler.
func (s *Server) AddLeaveHandler(handler PlayerCallback) {
	s.LeaveCallbacks = append(s.LeaveCallbacks, handler)
}

// calls all callbacks asyncronously, when a player joins.
func (s *Server) handleJoin(p Player) {
	for _, cb := range s.JoinCallbacks {
		cb(p)
	}
}

// calls all callbacks asyncronously, when a player leaves.
func (s *Server) handleLeave(p Player) {
	for _, cb := range s.LeaveCallbacks {
		cb(p)
	}
}
