package main

import (
	"context"
	"errors"
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
	config = configuration{}

	playerJoinRegex    = regexp.MustCompile(`\[server\]: server_join ClientID=([\d]{1,2}) addr=([^ ]+) '(.*)'$`)
	playerJoinWithClan = regexp.MustCompile(`\[server\]: server_join ClientID=([\d]{1,2}) addr=([^ ]+) '(.*)' '(.*)'$`)

	playerLeaveRegex   = regexp.MustCompile(`\[server\]: server_leave ClientID=([\d]{1,2}) addr=([^ ]+) '(.*)'$`)
	startVotekickRegex = regexp.MustCompile(`\[server\]: '([\d]{1,2}):(.*)' voted kick '([\d]{1,2}):(.*)' reason='(.{1,20})' cmd='(.*)' force=([\d])`)
	startSpecVoteRegex = regexp.MustCompile(`\[server\]: '([\d]{1,2}):(.*)' voted spectate '([\d]{1,2}):(.*)' reason='(.{1,20})' cmd='(.*)' force=([\d])`)
	executeRconCommand = regexp.MustCompile(`\[server\]: ClientID=([\d]{1,2}) rcon='(.*)'$`)
	chatRegex          = regexp.MustCompile(`\[chat\]: ([\d]+):[\d]+:(.{1,16}): (.*)$`)
	teamChatRegex      = regexp.MustCompile(`\[teamchat\]: ([\d]+):[\d]+:(.{1,16}): (.*)$`)
	whisperRegex       = regexp.MustCompile(`\[whisper\]: ([\d]+):[\d]+:(.{1,16}): (.*)$`)

	banAddRegex     = regexp.MustCompile(`\[net_ban\]: banned '(.*)' for ([\d]+) minute[s]? \((.*)\)$`)
	banRemoveRegex  = regexp.MustCompile(`\[net_ban\]: unbanned '(.*)'`)
	banExpiredRegex = regexp.MustCompile(`\[net_ban\]: ban '(.+)' expired$`)

	bansNumRegexp      = regexp.MustCompile(`\[net_ban\]: ([\d]+) ban[s]?$`)
	bansErrorRegex     = regexp.MustCompile(`\[net_ban\]: (.*error.*)$`)
	bansListEntryRegex = regexp.MustCompile(`\[net_ban\]: #([\d]+) '(.+)' banned for ([\d]+) minute[s]? \((.*)\)`)

	mutesAndVotebansRegex = regexp.MustCompile(`\[Server\]: (.*)`)

	moderatorMentions = regexp.MustCompile(`\[chat\]: .*(@moderators|@mods|@mod|@administrators|@admins|@admin).*`) // first plurals, then singular
)

func init() {

	config = configuration{
		EconPasswords:            make(map[address]password),
		ServerStates:             make(map[address]*server),
		ChannelAddress:           newChannelAddressMap(),
		DiscordModerators:        newUserSet(),
		SpiedOnPlayers:           newUserSet(),
		DiscordModeratorCommands: newCommandSet(),
		DiscordCommandQueue:      make(map[address]chan command),
	}

	env, err := godotenv.Read(".env")
	if err != nil {
		log.Fatal(err)
	}

	discordToken, ok := env["DISCORD_TOKEN"]

	if !ok || len(discordToken) == 0 {
		log.Fatal("error: no DISCORD_TOKEN specified")
	}
	config.DiscordToken = discordToken

	discordAdmin, ok := env["DISCORD_ADMIN"]
	if !ok || len(discordAdmin) == 0 {
		log.Fatal("error: no DISCORD_ADMIN specified")
	}
	config.DiscordAdmin = discordAdmin
	config.DiscordModerators.Add(discordAdmin)

	econServers, ok := env["ECON_SERVERS"]

	if !ok || len(econServers) == 0 {
		log.Fatal("error: no ECON_SERVERS specified")
	}

	econPasswords, ok := env["ECON_PASSWORDS"]
	if !ok || len(econPasswords) == 0 {
		log.Fatal("error: no ECON_PASSWORDS specified")
	}

	moderators, ok := env["DISCORD_MODERATORS"]
	if ok && len(moderators) > 0 {
		for _, moderator := range strings.Split(env["DISCORD_MODERATORS"], " ") {
			config.DiscordModerators.Add(moderator)
		}
	}

	commands, ok := env["DISCORD_MODERATOR_COMMANDS"]
	if ok {
		allowedCommands := strings.Split(commands, " ")
		for _, cmd := range allowedCommands {
			config.DiscordModeratorCommands.Add(cmd)
		}
	}

	moderatorRole, ok := env["DISCORD_MODERATOR_ROLE"]
	if ok && len(moderatorRole) > 0 {
		config.DiscordModeratorRole = moderatorRole
	}

	servers := strings.Split(econServers, " ")
	passwords := strings.Split(econPasswords, " ")

	// fill list with first password
	if len(passwords) == 1 && len(servers) > 1 {
		for i := 1; i < len(servers); i++ {
			passwords = append(passwords, passwords[0])
		}
	} else if len(passwords) != len(servers) {
		log.Fatal("ECON_SERVERS and ECON_PASSWORDS mismatch")
	} else if len(servers) == 0 {
		log.Fatal("No ECON_SERVERS and/or ECON_PASSWORDS specified.")
	}

	for idx, addr := range servers {
		config.EconPasswords[address(addr)] = password(passwords[idx])
	}

	for _, addr := range servers {
		config.ServerStates[address(addr)] = &server{}
		config.DiscordCommandQueue[address(addr)] = make(chan command)
	}

	logLevel, ok := env["LOG_LEVEL"]
	if ok && len(logLevel) > 0 {
		level, err := strconv.Atoi(logLevel)
		if err != nil {
			log.Printf("Invalid value for LOG_LEVEL: %s", logLevel)
		} else {
			config.LogLevel = level
		}
	}

	log.Printf("\n%s", config.String())
}

func parseCommandLine(cmd string) (line string, send bool, err error) {
	args := strings.Split(cmd, " ")
	if len(args) > 0 {
		// help is not part of the commands
		if args[0] == "help" {
			sb := strings.Builder{}
			sb.WriteString("Available Commands: \n")
			sb.WriteString("```")
			for _, cmd := range config.DiscordModeratorCommands.Commands() {
				sb.WriteString(fmt.Sprintf("?%s\n", cmd))
			}
			sb.WriteString("```")

			sb.WriteString("Moderators:\n")
			sb.WriteString("```")
			for _, moderator := range config.DiscordModerators.Users() {
				sb.WriteString(fmt.Sprintf("%s\n", moderator))
			}
			sb.WriteString("```")

			err = errors.New(sb.String())
			return
		}

		// all allowed commands
		if !config.DiscordModeratorCommands.Contains(args[0]) {
			err = fmt.Errorf("access to command %q denied", args[0])
			return
		}
	}

	line = cmd
	send = true
	return
}

func parseEconLine(line string, server *server) (result string, send bool) {

	if strings.Contains(line, "[server]") {
		// contains all commands that contain [server] as prefix.

		matches := playerJoinRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 3) {

			id, _ := strconv.Atoi(matches[1])
			name := matches[3]
			addr := address(matches[2])
			server.join(id, player{Name: name, ID: id, Address: addr})

			result = fmt.Sprintf("[server]: '%s' joined the server with id %d", name, id)
			if config.LogLevel >= 2 {
				send = true
			}
			return
		}

		matches = playerJoinWithClan.FindStringSubmatch(line)
		if len(matches) == (1 + 4) {

			id, _ := strconv.Atoi(matches[1])
			name := matches[3]
			clan := matches[4]
			addr := address(matches[2])
			server.join(id, player{Name: name, Clan: clan, ID: id, Address: addr})

			result = fmt.Sprintf("[server]: '%s' joined the server with id %d", name, id)
			if config.LogLevel >= 2 {
				send = true
			}
			return
		}

		matches = playerLeaveRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 3) {
			id, _ := strconv.Atoi(matches[1])
			// ip := matches[2]
			name := matches[3]
			server.leave(id)

			result = fmt.Sprintf("[server]: '%s' left the server, id was %d", name, id)
			if config.LogLevel >= 2 {
				send = true
			}
			return
		}

		matches = startVotekickRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 7) {
			kickingID, _ := strconv.Atoi(matches[1])
			kickingName := matches[2]

			kickedID, _ := strconv.Atoi(matches[3])
			kickedName := matches[4]

			reason := matches[5]

			result = fmt.Sprintf("**[kickvote]**: '%d:%s' started to kick '%d:%s' with reason '%s'", kickingID, kickingName, kickedID, kickedName, reason)
			send = true
			return
		}

		matches = startSpecVoteRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 7) {
			votingID, _ := strconv.Atoi(matches[1])
			votingName := matches[2]

			votedID, _ := strconv.Atoi(matches[3])
			votedName := matches[4]

			reason := matches[5]

			result = fmt.Sprintf("**[specvote]**: '%d:%s' wants to move '%d:%s' to spectators with reason '%s'", votingID, votingName, votedID, votedName, reason)
			send = true
			return
		}

		matches = executeRconCommand.FindStringSubmatch(line)
		if len(matches) == (1 + 2) {
			adminID, _ := strconv.Atoi(matches[1])
			name := server.Player(adminID).Name
			command := matches[2]

			result = fmt.Sprintf("**[rcon]**: '%s' command='%s'", name, command)
			send = true
			return
		}

		return
	} else if strings.Contains(line, "[net_ban]") {

		matches := banAddRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 3) {
			ip := matches[1]

			minutes, _ := strconv.Atoi(matches[2])
			duration := time.Minute * time.Duration(minutes)
			reason := matches[3]

			result = fmt.Sprintf("**[bans]**: '%s' banned for %s with reason: '%s'", ip, duration, reason)
			send = true
			return
		}

		matches = banExpiredRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 1) {
			ip := matches[1]
			result = fmt.Sprintf("[bans]: ban of '%s' expired", ip)
			send = true
			return
		}

		matches = bansNumRegexp.FindStringSubmatch(line)
		if len(matches) == (1 + 1) {
			numberOfBans, _ := strconv.Atoi(matches[1])
			result = fmt.Sprintf("[banlist]: %d ban(s)", numberOfBans)
			send = true
			return
		}

		matches = bansListEntryRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 4) {
			index, _ := strconv.Atoi(matches[1])
			ip := matches[2]

			minutes, _ := strconv.Atoi(matches[3])
			duration := time.Minute * time.Duration(minutes)
			reason := matches[4]
			result = fmt.Sprintf("[banlist]: idx=%-2d '%s' %-9s (%s)", index, ip, duration, reason)
			send = true
			return
		}

		matches = bansErrorRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 1) {
			errorMsg := matches[1]
			result = fmt.Sprintf("**[error]**: %s", errorMsg)
			send = true
			return
		}

		matches = banRemoveRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 1) {
			ip := matches[1]
			result = fmt.Sprintf("[bans]: unbanned %s", ip)
			send = true
			return
		}

		return
	}

	matches := chatRegex.FindStringSubmatch(line)
	if len(matches) == (1 + 3) {
		id, _ := strconv.Atoi(matches[1])
		name := matches[2]
		text := matches[3]

		result = fmt.Sprintf("[chat]: %d:'%s': %s", id, name, text)
		send = true
		return
	}

	matches = teamChatRegex.FindStringSubmatch(line)
	if len(matches) == (1 + 3) {
		id, _ := strconv.Atoi(matches[1])
		name := matches[2]
		text := matches[3]

		result = fmt.Sprintf("[teamchat]: %d:'%s': %s", id, name, text)
		send = true
		return
	}

	matches = whisperRegex.FindStringSubmatch(line)
	if len(matches) == (1 + 3) {
		id, _ := strconv.Atoi(matches[1])
		name := matches[2]
		message := matches[3]

		if config.LogLevel >= 1 || config.SpiedOnPlayers.Contains(name) {
			result = fmt.Sprintf("[whisper] %d:'%s': %s", id, name, message)
			send = true
		}
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
	defer config.Close()

	dg, err := discordgo.New("Bot " + config.DiscordToken)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		if strings.HasPrefix(m.Content, "?") {
			if !config.DiscordModerators.Contains(m.Author.String()) {
				return
			}

			addr, ok := config.ChannelAddress.Get(discordChannel(m.ChannelID))
			if !ok {
				log.Printf("Request from invalid channel by user %s", m.Author.String())
				return
			}
			cmd := m.Content[1:]

			// handle status from cache data
			if cmd == "status" {
				if config.DiscordModeratorCommands.Contains(cmd) {
					players := config.ServerStates[addr].Status()

					if len(players) == 0 {
						s.ChannelMessageSend(m.ChannelID, "There are currently no players online.")
						return
					}

					sb := strings.Builder{}
					sb.WriteString("```")
					for _, p := range players {
						sb.WriteString(fmt.Sprintf("id=%-2d %s\n", p.ID, p.Name))
					}
					sb.WriteString("```")

					_, err := s.ChannelMessageSend(m.ChannelID, sb.String())
					if err != nil {
						log.Printf("error while sending status message: %s", err.Error())
					}

					return
				}

				_, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Access to the command %q denied.", cmd))
				if err != nil {
					log.Printf("error while sending status message: %s", err.Error())
				}
			}

			// send other messages this way
			config.DiscordCommandQueue[addr] <- command{Author: m.Author.String(), Command: cmd}
			return

		}

		if !strings.HasPrefix(m.Content, "#") {
			return
		}

		if m.Author.String() != config.DiscordAdmin {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Not an admin, only '%s' is allowed to execute commands.", config.DiscordAdmin))
			return
		}

		args := strings.Split(m.Content, " ")

		if len(args) < 1 {
			return
		}

		command := args[0][1:]
		arguments := strings.Join(args[1:], " ")

		switch command {
		case "add":
			user := strings.Trim(arguments, " \n")
			config.DiscordModerators.Add(user)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Added %q to moderators", user))
			return
		case "remove":
			user := strings.Trim(arguments, " \n")
			config.DiscordModerators.Remove(user)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Removed %q from moderators", user))
			return
		case "purge":
			config.DiscordModerators.Reset()
			config.DiscordModerators.Add(config.DiscordAdmin)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Purged all moderators except %q", config.DiscordAdmin))
			return
		case "clean":
			msg, _ := s.ChannelMessageSend(m.ChannelID, "starting channel cleanup...")

			initialID := msg.ChannelID
			for msgs, err := s.ChannelMessages(initialID, 100, msg.ID, "", ""); len(msgs) > 0 && err == nil; {
				if err != nil {
					log.Printf("error while cleaning up a channel: %s\n", err.Error())
					break
				}
				if len(msgs) == 0 {
					break
				}

				msgIDs := make([]string, 0, len(msgs))

				for _, msg := range msgs {
					msgIDs = append(msgIDs, msg.ID)
				}

				delErr := s.ChannelMessagesBulkDelete(msg.ChannelID, msgIDs)
				if delErr != nil {
					log.Printf("error while trying to bulk delete %d messages: %s", len(msgIDs), delErr)
					s.ChannelMessageSend(msg.ChannelID, "The bot does not have enough permissions to cleanup the channel.")

					// delete initial message in any case.
					s.ChannelMessageDelete(msg.ChannelID, msg.ID)
					return
				}

				initialID = msgIDs[len(msgIDs)-1]
			}

			s.ChannelMessageDelete(msg.ChannelID, msg.ID)
			s.ChannelMessageSend(m.ChannelID, "cleanup done!")

		case "moderate":

			if len(args) < 2 {
				break
			}
			addr := address(args[1])
			pass, ok := config.EconPasswords[addr]
			currentChannel := discordChannel(m.ChannelID)

			if !ok {
				s.ChannelMessageSend(m.ChannelID, "unknown server address")
				return
			}

			// handle single time registration with a discord channel
			if config.ChannelAddress.AlreadyRegistered(addr) {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("The address %s is already registered with a channel.", addr))
				return
			}
			config.ChannelAddress.Set(currentChannel, addr)

			// start routine to listen to specified server.
			go func(ctx context.Context, channelID, initialMessageID string, s *discordgo.Session, addr address, pass password) {
				// channel - server association
				defer config.ChannelAddress.RemoveAddress(addr)

				// sub goroutines
				routineContext, routineCancel := context.WithCancel(ctx)
				defer routineCancel()

				// econ connection
				conn, err := econ.DialTo(string(addr), string(pass))
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, err.Error())
					return
				}
				defer conn.Close()

				// cleanup all messages before the initial message
				go func(channelID, initialMessageID string, s *discordgo.Session) {
					for {
						select {
						case <-routineContext.Done():
							return
						default:
							messages, err := s.ChannelMessages(channelID, 100, initialMessageID, "", "")
							if err != nil {
								log.Printf("error on purging previous channel messages: %s", err.Error())
								continue
							}
							if len(messages) == 0 {
								log.Println("finished cleaning up old messages.")
								return
							}
							ids := make([]string, 0, 100)

							for _, msg := range messages {
								ids = append(ids, msg.ID)
							}

							err = s.ChannelMessagesBulkDelete(channelID, ids)
							if err != nil {
								log.Printf("error on bulk deleting previous messages: %s", err.Error())
								return
							}

							log.Printf("deleted %d old messages.", len(messages))
						}
					}
				}(channelID, initialMessageID, s)

				// start channel history cleanup
				go func(channelID, initialMessageID string, s *discordgo.Session) {

					if err != nil {
						log.Println(err.Error())
						return
					}

					for {
						timer := time.NewTimer(2 * time.Minute)

						select {
						case <-routineContext.Done():
							log.Printf("closing main routine of: %s\n", addr)
							return
						case <-timer.C:
							messages, err := s.ChannelMessages(channelID, 100, "", initialMessageID, "")
							if err != nil {
								log.Printf("error on cleanup: %s", err.Error())
								continue
							}

							cleanedUp := 0
							for _, message := range messages {

								created, err := message.Timestamp.Parse()
								if err != nil {
									log.Printf("error parsing message: %s", err.Error())
									continue
								}

								// TODO: make variable
								if time.Since(created) > 24*time.Hour {

									err := s.ChannelMessageDelete(channelID, message.ID)
									if err != nil {
										log.Printf("Error occurred while deleting messages: %s", err.Error())
									} else {
										cleanedUp++
									}
								}
							}
							log.Printf("cleaned up %d of history messages", cleanedUp)
						}
					}
				}(channelID, initialMessageID, s)

				go func(addr address, channelID string, s *discordgo.Session) {
					for {
						select {
						case <-routineContext.Done():
							log.Printf("closing command queue routine of: %s\n", addr)
							return
						case cmd, ok := <-config.DiscordCommandQueue[addr]:
							if !ok {
								return
							}

							lineToExecute, send, err := parseCommandLine(cmd.Command)
							if err != nil {
								s.ChannelMessageSend(channelID, err.Error())
								continue
							}
							if send {
								conn.WriteLine(fmt.Sprintf("echo [Discord] user '%s' executed rcon '%s'", strings.Replace(cmd.Author, "#", "_", -1), lineToExecute))
								conn.WriteLine(lineToExecute)
							}
						}
					}
				}(addr, channelID, s)

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
						log.Printf("closing econ line parsing routine routine of: %s\n", addr)
						return
					case line := <-result:
						// if read avalable, parse and if necessary, send
						line, send := parseEconLine(line, config.ServerStates[addr])
						if send {

							// check for moderator mention
							matches := moderatorMentions.FindStringSubmatch(line)
							if len(matches) == (1 + 1) {

								// there is a role configured
								if len(config.DiscordModeratorRole) > 0 {
									mention := matches[1]

									roles, _ := s.GuildRoles(m.GuildID)

									mentionReplace := ""
									for _, role := range roles {
										if strings.Contains(role.Name, config.DiscordModeratorRole) {
											mentionReplace = role.Mention()
											break
										}
									}

									if len(mentionReplace) > 0 {
										line = strings.ReplaceAll(line, mention, mentionReplace)
									}
								}
							}

							_, err := s.ChannelMessageSend(channelID, line)
							if err != nil {
								log.Printf("error while sending line: %s\n", err.Error())
							}
						}

					}
				}

			}(ctx, m.ChannelID, m.ID, s, addr, pass)

			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Started listening to server %s", addr))
			return
		case "spy":
			nickname := strings.Trim(arguments, " \n")
			config.SpiedOnPlayers.Add(nickname)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Spying on %q ", nickname))
			return
		case "unspy":
			nickname := strings.Trim(arguments, " \n")
			config.SpiedOnPlayers.Remove(nickname)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Stopped spying on %q", nickname))
			return
		case "purgespy":
			config.SpiedOnPlayers.Reset()
			s.ChannelMessageSend(m.ChannelID, "Purged all spied on players.")

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

	log.Println("Shutting down, please wait...")
}
