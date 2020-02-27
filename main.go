package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/jxsl13/twapi/econ"
)

var (
	playerJoinRegex    = regexp.MustCompile(`\[server\]: server_join ClientID=([\d]{1,2}) addr=([^ ]+) '(.*)'$`)
	playerLeaveRegex   = regexp.MustCompile(`\[server\]: server_leave ClientID=([\d]{1,2}) addr=([^ ]+) '(.*)'$`)
	startVotekickRegex = regexp.MustCompile(`\[server\]: '([\d]{1,2}):(.*)' voted kick '([\d]{1,2}):(.*)' reason='(.{1,20})' cmd='(.*)' force=([\d])`)
	executeRconCommand = regexp.MustCompile(`\[server\]: ClientID=([\d]{1,2}) rcon='(.*)'$`)
	chatRegex          = regexp.MustCompile(`\[chat\]: ([\d]+):[\d]+:([^:]{1,16}):(.*)$`)

	bansListRegex         = regexp.MustCompile(`\[net_ban\]: (.*)`)
	mutesAndVotebansRegex = regexp.MustCompile(`\[Server\]: (.*)`)
)

type command struct {
	Author  string
	Command string
}

func parseCommandLine(cmd string) (line string, send bool) {
	line = cmd
	send = true
	return
}

func parseEconLine(line string, server *server) (result string, send bool) {

	if strings.Contains(line, "[server]") {
		matches := playerJoinRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 3) {
			id, _ := strconv.Atoi(matches[1])
			name := matches[3]
			address := matches[2]
			server.join(id, player{Name: name, ID: id, Address: address})

			result = fmt.Sprintf("[server]: '%s' joined the server with id %d", name, id)
			send = true
			return
		}

		matches = playerLeaveRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 3) {
			id, _ := strconv.Atoi(matches[1])
			name := matches[3]
			server.leave(id)

			result = fmt.Sprintf("[server]: '%s' left the server, id was %d", name, id)
			send = true
			return
		}

		matches = startVotekickRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 7) {
			kickingID, _ := strconv.Atoi(matches[1])
			kickingName := matches[2]

			kickedID, _ := strconv.Atoi(matches[3])
			kickedName := matches[4]

			reason := matches[5]

			result = fmt.Sprintf("[kickvote]: '%d:%s' started to kick '%d:%s' with reason '%s'", kickingID, kickingName, kickedID, kickedName, reason)
			send = true
			return
		}
	}

	matches := chatRegex.FindStringSubmatch(line)
	if len(matches) == (1 + 3) {
		id, _ := strconv.Atoi(matches[1])
		name := matches[2]
		text := matches[3]

		result = fmt.Sprintf("[chat]: '%d:%s': %s", id, name, text)
		send = true
		return
	}

	matches = executeRconCommand.FindStringSubmatch(line)
	if len(matches) == (1 + 2) {
		adminID, _ := strconv.Atoi(matches[1])
		name := server.PlayerName(adminID)
		command := matches[2]

		result = fmt.Sprintf("[rcon]: '%s' command='%s'", name, command)
		send = true
		return
	}

	matches = bansListRegex.FindStringSubmatch(line)
	if len(matches) == (1 + 1) {
		text := matches[1]
		result = fmt.Sprintf("[net_ban]: %s", text)
		send = true
		return
	}

	matches = mutesAndVotebansRegex.FindStringSubmatch(line)
	if len(matches) == (1 + 1) {
		text := matches[1]
		result = fmt.Sprintf("[server]: %s", text)
		send = true
		return
	}

	return
}

func main() {
	env, err := godotenv.Read(".env")
	if err != nil {
		log.Fatal(err)
	}

	discordToken := env["DISCORD_TOKEN"]

	if discordToken == "" {
		log.Fatal("error: no DISCORD_TOKEN specified")
	}

	discordAdmin := env["DISCORD_ADMIN"]

	if discordToken == "" {
		log.Fatal("error: no DISCORD_ADMINS specified")
	}

	econServersEnv := env["ECON_SERVERS"]

	if econServersEnv == "" {
		log.Fatal("error: no ECON_SERVERS specified")
	}

	econPasswordsEnv := env["ECON_PASSWORDS"]

	if econPasswordsEnv == "" {
		log.Fatal("error: no ECON_PASSWORDS specified")
	}

	moderators := newUserSet()
	moderators.Add(discordAdmin)

	for _, moderator := range strings.Split(env["DISCORD_MODERATORS"], " ") {
		moderators.Add(moderator)
	}

	econServers := strings.Split(econServersEnv, " ")
	econPasswords := strings.Split(econPasswordsEnv, " ")

	// fill list with first password
	if len(econPasswords) == 1 && len(econServers) > 1 {
		for i := 1; i < len(econServers); i++ {
			econPasswords = append(econPasswords, econPasswords[0])
		}
	} else if len(econPasswords) != len(econServers) {
		log.Fatal("ECON_SERVERS and ECON_PASSWORDS mismatch")
	} else if len(econServers) == 0 {
		log.Fatal("No ECON_SERVERS specified.")
	}

	passwordCache := make(map[string]string, len(econServers))

	for idx, address := range econServers {
		passwordCache[address] = econPasswords[idx]
	}

	serverStateCache := make(map[string]*server, len(passwordCache))

	channelAddressMap := newChannelAddressMap()
	econCommandChannels := make(map[string]chan command)

	for _, address := range econServers {
		serverStateCache[address] = &server{}
		econCommandChannels[address] = make(chan command)
	}

	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		if strings.HasPrefix(m.Content, "?") {
			if !moderators.Contains(m.Author.String()) {
				return
			}

			addr, ok := channelAddressMap.Get(m.ChannelID)
			if !ok {
				log.Printf("Request from invalid channel by user %s", m.Author.String())
				return
			}
			cmd := m.Content[1:]

			// handle status from cache data
			if cmd == "status" {
				players := serverStateCache[addr].Status()

				if len(players) == 0 {
					s.ChannelMessageSend(m.ChannelID, "There are currently no players online.")
					return
				}

				sb := strings.Builder{}
				sb.WriteString("```")
				for _, p := range players {
					sb.WriteString(fmt.Sprintf("id=%-2d %s", p.ID, p.Name))
				}
				sb.WriteString("```")

				_, err := s.ChannelMessageSend(m.ChannelID, sb.String())
				if err != nil {
					log.Printf("error while sending status message: %s", err.Error())
				}

				return
			}

			// send other messages this way
			econCommandChannels[addr] <- command{Author: m.Author.String(), Command: cmd}
			return

		}

		if !strings.HasPrefix(m.Content, "#") {
			return
		}

		if m.Author.String() != discordAdmin {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Not an admin, only '%s' is allowed to execute commands.", discordAdmin))
			return
		}

		args := strings.Split(m.Content, " ")

		if len(args) < 1 {
			return
		}

		command := args[0][1:]

		switch command {
		case "add":
			moderators.Add(strings.Trim(command, " \n"))
			return
		case "remove":
			moderators.Remove(strings.Trim(command, " \n"))
			return
		case "purge":
			moderators.Reset()
			moderators.Add(discordAdmin)
			return
		case "moderate":

			if len(args) < 2 {
				break
			}
			addr := args[1]
			password, ok := passwordCache[addr]

			if !ok {
				s.ChannelMessageSend(m.ChannelID, "unknown server address")
				return
			}

			// handle single time registration with a discord channel
			if channelAddressMap.AlreadyRegistered(addr) {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("The address %s is already registered with a channel.", addr))
				return
			}
			channelAddressMap.Set(m.ChannelID, addr)

			// start routine to listen to specified server.
			go func(ctx context.Context, channelID, initialMessageID string, s *discordgo.Session, address, password string) {
				conn, err := econ.DialTo(address, password)
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, err.Error())
					return
				}
				defer conn.Close()

				// start channel cleanup
				go func(channelID, initialMessageID string, s *discordgo.Session) {

					if err != nil {
						log.Println(err.Error())
						return
					}

					for {
						timer := time.NewTimer(2 * time.Minute)

						select {
						case <-ctx.Done():
							return
						case <-timer.C:
							messages, err := s.ChannelMessages(channelID, 100, "", initialMessageID, "")
							if err != nil {
								log.Printf("error on cleanup: %s", err.Error())
								continue
							}

							log.Printf("retrieved %d messages", len(messages))
							cleanedUp := 0
							for _, message := range messages {

								created, err := message.Timestamp.Parse()
								if err != nil {
									log.Printf("error parsing message: %s", err.Error())
									continue
								}

								if time.Since(created) > 24*time.Hour {

									err := s.ChannelMessageDelete(channelID, message.ID)
									if err != nil {
										log.Printf("Error occurred while deleting messages: %s", err.Error())
									} else {
										cleanedUp++
									}
								}
							}
							log.Printf("cleaned up %d messages", cleanedUp)
						}
					}
				}(channelID, initialMessageID, s)

				go func() {
					for {
						select {
						case <-ctx.Done():
							return
						case cmd, ok := <-econCommandChannels[address]:
							if !ok {
								return
							}

							lineToExecute, send := parseCommandLine(cmd.Command)
							if send {
								conn.WriteLine(fmt.Sprintf("echo User '%s' executed rcon '%s'", strings.Replace(cmd.Author, "#", "_", -1), lineToExecute))
								conn.WriteLine(lineToExecute)
							}

						}
					}
				}()

				// handle econ line parsing
				result := make(chan string)
				defer close(result)

				for {

					// start routine for waiting for line
					go func() {
						line, err := conn.ReadLine()
						if err != nil {
							// intended
							return
						}
						result <- line
					}()

					// wait for read or abort
					select {
					case <-ctx.Done():
						return
					case line := <-result:
						// if read avalable, parse and if necessary, send
						line, send := parseEconLine(line, serverStateCache[address])
						if send {
							_, err := s.ChannelMessageSend(channelID, line)
							if err != nil {
								log.Printf("error while sending line: %s\n", err.Error())
							}
						}

					}
				}

			}(ctx, m.ChannelID, m.ID, s, addr, password)

			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Started listening to server %s", addr))
			return
		}

		s.ChannelMessageSend(m.ChannelID, "invalid command")
		return

	})

	err = dg.Open()
	if err != nil {
		log.Fatalf("error: could not establish a connection to the discord api, please check your credentials")
	}
	defer dg.Close()

	// Wait here until CTRL-C or other term signal is received.
	log.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	cancel()

	for _, address := range econServers {
		close(econCommandChannels[address])
	}
	log.Println("Shutting down, please wait...")
}
