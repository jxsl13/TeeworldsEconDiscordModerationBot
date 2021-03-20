package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jxsl13/twapi/econ"
)

var (
	// [2020-05-22 23:01:09][client_enter]: id=0 addr=192.168.178.25:64139 version=1796 name='MisterFister:(' clan='FistingTea`' country=-1
	// 0: full 1: timestamp 2: log level 3: log line
	initialLoglevelRegex = regexp.MustCompile(`\[([\d -:]+)\]\[([^:]+)\]: (.+)$`)

	// logLevel: server
	startVotekickRegex = regexp.MustCompile(`'([\d]{1,2}):(.*)' voted kick '([\d]{1,2}):(.*)' reason='(.{1,20})' cmd='(.*)' force=([\d])`)
	startSpecVoteRegex = regexp.MustCompile(`'([\d]{1,2}):(.*)' voted spectate '([\d]{1,2}):(.*)' reason='(.{1,20})' cmd='(.*)' force=([\d])`)
	startOptionVote    = regexp.MustCompile(`'([\d]{1,2}):(.*)' voted option '(.+)' reason='(.{1,20})' cmd='(.+)' force=([\d])`)

	// 0: full 1: ID 2: rank
	// logLevel: server
	loginRconRegex     = regexp.MustCompile(`ClientID=(\d+) authed \((.*)\)`)
	executeRconCommand = regexp.MustCompile(`ClientID=(\d+) rcon='(.*)'$`)

	// logLevel: chat
	chatRegex = regexp.MustCompile(`([\d]+):[\d]+:(.{1,16}): (.*)$`)
	// logLevel: teamchat
	teamChatRegex = regexp.MustCompile(`([\d]+):[\d]+:(.{1,16}): (.*)$`)
	// logLevel: whisper
	whisperRegex = regexp.MustCompile(`([\d]+):[\d]+:(.{1,16}): (.*)$`)

	// logLevel: net_ban
	bansErrorRegex = regexp.MustCompile(`(.*error.*)$`)

	moderatorMentions = regexp.MustCompile(`\[chat\]: [\d]+:'.*': .*(@moderators|@mods|@mod|@administrators|@admins|@admin).*$`) // first plurals, then singular

	formatedSpecVoteKickStringRegex = regexp.MustCompile(`\*\*\[.*vote.*\]\*\*\: ([\d]+):'(.{0,20})' [^\d]{12,15} ([\d]+):'(.{0,20})'( to spectators)? with reason '(.+)'$`)

	formatedBanRegex = regexp.MustCompile(`\*\*\[bans\]\*\*: '(.*)' banned for (.*) with reason: '(.*)'$`)

	// logLevel: server
	forcedYesRegex = regexp.MustCompile(`forcing vote yes$`)
	forcedNoRegex  = regexp.MustCompile(`forcing vote no$`)
)

func serverRoutine(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, addr Address, pass password) {
	// sub goroutines
	routineContext, routineCancel := context.WithCancel(ctx)
	defer routineCancel()

	// channel - server association
	defer config.ChannelAddress.RemoveAddress(addr)
	defer s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Stopped listening to server %s", addr))
	initialMessageID := m.ID

	config.AnnouncemenServers[addr] = NewAnnouncementServer(routineContext, config.DiscordCommandQueue[addr])

	// econ connection
	conn, err := econ.DialTo(string(addr), string(pass))
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, err.Error())
		return
	}
	defer conn.Close()

	// cleanup all messages before the initial message
	go cleanupRoutine(routineContext, s, m.ChannelID, initialMessageID)

	// start channel history cleanup
	go logCleanupRoutine(routineContext, s, m.ChannelID, initialMessageID, addr)

	// execution of discord commands
	go commandQueueRoutine(routineContext, s, m.ChannelID, conn, addr)

	// handle econ line parsing
	result := make(chan string)
	defer close(result)

	// start routine for waiting for line
	go func(ctx context.Context, conn *econ.Conn) {
		// set log level of server. in order to parse it directly after connection.
		conn.WriteLine("ec_output_level 2")

		for {
			select {
			case <-ctx.Done():
				log.Println("Closing econ reader routine of:", addr, " : ", err.Error())
				return
			default:
				line, err := conn.ReadLine()
				if err != nil {
					s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("econ read error: %s", err.Error()))
					continue
				}
				result <- line
			}
		}

	}(ctx, conn)

	for {

		// wait for read or abort
		select {
		case <-ctx.Done():
			log.Printf("closing econ line parsing routine of: %s\n", addr)
			return
		case line := <-result:
			// if read avalable, parse and if necessary, send
			fmtLine, send := parseEconLine(line, config.ServerStates[addr])

			if send {
				// check for moderator mention
				fmtLine = replaceModeratorMentions(s, m, fmtLine)

				msg, err := s.ChannelMessageSend(m.ChannelID, fmtLine)
				if err != nil {
					log.Printf("error while sending line: %s\n", err.Error())
				}

				handleMessageReactions(routineContext, s, msg, line, fmtLine)
			}

		}
	}
}

func cleanupRoutine(routineContext context.Context, s *discordgo.Session, channelID, initialMessageID string) {
	defer log.Println("finished cleaning up old messages.")

	cleanedUpMessages := 0

loop:
	for {
		select {
		case <-routineContext.Done():
			break loop
		default:
			messages, err := s.ChannelMessages(channelID, 100, initialMessageID, "", "")
			if err != nil {
				log.Printf("error on purging previous channel messages: %s", err.Error())
				continue
			}
			if len(messages) == 0 {
				break loop
			}
			bulkIDs := make([]string, 0, 100)
			manualIDs := make([]string, 0, 100)

			const bulkDelay = 14*24*time.Hour - time.Minute
			for _, msg := range messages {
				timestamp, _ := msg.Timestamp.Parse()

				if time.Since(timestamp) >= bulkDelay {
					bulkIDs = append(bulkIDs, msg.ID)
				} else {
					manualIDs = append(manualIDs, msg.ID)
				}
			}

			err = s.ChannelMessagesBulkDelete(channelID, bulkIDs)
			if err != nil {
				log.Printf("error on bulk deleting old messages: %s", err.Error())
				if len(manualIDs) == 0 {
					break loop
				}
			}

			for _, id := range manualIDs {
				err := s.ChannelMessageDelete(channelID, id)
				if err != nil {
					log.Printf("error on deleting old message: %s", err.Error())
					break loop
				}
			}

			cleanedUpMessages += len(messages)
		}
	}

	log.Printf("deleted %d old messages.", cleanedUpMessages)
}

func logCleanupRoutine(routineContext context.Context, s *discordgo.Session, channelID, initialMessageID string, addr Address) {

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
		}
	}
}

func commandQueueRoutine(routineContext context.Context, s *discordgo.Session, channelID string, conn *econ.Conn, addr Address) {

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
				escapedNick := strings.ReplaceAll(cmd.Author, "#", "_")
				logLine := fmt.Sprintf("echo [Discord] user '%s' executed rcon '%s'", escapedNick, lineToExecute)
				conn.WriteLine(logLine)
				conn.WriteLine(lineToExecute)
			}
		}
	}
}

func parseCommandLine(cmd string) (line string, send bool, err error) {
	args := strings.Split(cmd, " ")
	if len(args) < 1 {
		return
	}

	return cmd, true, nil
}

func parseEconLine(line string, server *Server) (result string, send bool) {

	matches := initialLoglevelRegex.FindStringSubmatch(line)
	if len(matches) != 4 {
		return "", false
	}

	logLevel := matches[2]
	logLine := matches[3]

	switch logLevel {
	case "client_enter", "client_drop":
		if consumed, fmtLine := server.ParseLine(logLevel, logLine, config.JoinNotify); consumed {
			result = fmtLine
			if fmtLine != "" {
				send = true
			}
		}
		return
	case "server":
		if consumed, fmtLine := server.ParseLine(logLevel, logLine, config.JoinNotify); consumed {
			result = fmtLine
			if fmtLine != "" {
				send = true
			}
			return
		}

		var matches []string
		matches = startOptionVote.FindStringSubmatch(logLine)
		if len(matches) == (1 + 7) {
			votingID, _ := strconv.Atoi(matches[1])
			votingName := matches[2]

			optionName := matches[3]
			reason := matches[4]

			forced := matches[6]

			if forced == "1" {
				forced = "/forced"
			} else {
				forced = ""
			}

			result = fmt.Sprintf("**[optionvote%s]**: %d:'%s' voted option '%s' with reason '%s'", forced, votingID, Escape(votingName), Escape(optionName), Escape(reason))
			send = true
			return
		}

		matches = startVotekickRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 7) {
			kickingID, _ := strconv.Atoi(matches[1])
			kickingName := matches[2]

			kickedID, _ := strconv.Atoi(matches[3])
			kickedName := matches[4]

			reason := matches[5]
			forced := matches[7]

			if forced == "1" {
				forced = "/forced"
			} else {
				forced = ""
			}

			result = fmt.Sprintf("**[kickvote%s]**: %d:'%s' started to kick %d:'%s' with reason '%s'", forced, kickingID, Escape(kickingName), kickedID, Escape(kickedName), Escape(reason))
			send = true
			return
		}

		matches = startSpecVoteRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 7) {
			votingID, _ := strconv.Atoi(matches[1])
			votingName := matches[2]

			votedID, _ := strconv.Atoi(matches[3])
			votedName := matches[4]

			reason := matches[5]
			forced := matches[7]

			if forced == "1" {
				forced = "/forced"
			} else {
				forced = ""
			}

			result = fmt.Sprintf("**[specvote%s]**: %d:'%s' wants to move %d:'%s' to spectators with reason '%s'", forced, votingID, Escape(votingName), votedID, Escape(votedName), Escape(reason))
			send = true
			return
		}

		matches = forcedYesRegex.FindStringSubmatch(logLine)
		if len(matches) == 1 {
			result = "**[server]**: Forced Yes"
			send = true
			return
		}

		matches = forcedNoRegex.FindStringSubmatch(logLine)
		if len(matches) == 1 {
			result = "**[server]**: Forced No"
			send = true
			return
		}

		matches = loginRconRegex.FindStringSubmatch(logLine)
		if len(matches) == 3 {
			id, _ := strconv.Atoi(matches[1])
			rank := matches[2]

			result = fmt.Sprintf("**[rcon]**: '%s' authed as **%s**", Escape(server.Player(id).Name), rank)
			send = true
			return
		}

		matches = executeRconCommand.FindStringSubmatch(logLine)
		if len(matches) == (1 + 2) {
			adminID, _ := strconv.Atoi(matches[1])
			name := server.Player(adminID).Name
			command := matches[2]

			result = fmt.Sprintf("**[rcon]**: '%s' command='%s'", Escape(name), Escape(command))
			send = true
			return
		}

		return
	case "net_ban":
		if consumed, fmtLine := server.ParseLine(logLevel, logLine, config.JoinNotify); consumed {
			result = fmtLine
			if fmtLine != "" {
				send = true
			}
			return
		}

		matches := bansErrorRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 1) {
			errorMsg := matches[1]
			result = fmt.Sprintf("**[error]**: %s", errorMsg)
			send = true
		}

		return
	case "chat":
		matches := chatRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 3) {
			id, _ := strconv.Atoi(matches[1])
			name := matches[2]
			text := matches[3]

			result = fmt.Sprintf("[chat]: %d:'%s': %s", id, Escape(name), Escape(text))
			send = true
		}
		return
	case "teamchat":
		matches = teamChatRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 3) {
			id, _ := strconv.Atoi(matches[1])
			name := matches[2]
			text := matches[3]

			result = fmt.Sprintf("[teamchat]: %d:'%s': %s", id, Escape(name), Escape(text))
			send = true
		}
		return
	case "whisper":
		matches = whisperRegex.FindStringSubmatch(logLine)
		if len(matches) == (1 + 3) {
			id, _ := strconv.Atoi(matches[1])
			name := matches[2]
			message := matches[3]

			if config.LogLevel >= 1 || config.SpiedOnPlayers.Contains(name) {
				result = fmt.Sprintf("[whisper] %d:'%s': %s", id, Escape(name), Escape(message))
				send = true
			}
		}
		return
	case "Server":
		result = fmt.Sprintf("[server]: %s", Escape(logLine))
		send = true
		return
	}
	return
}

func replaceModeratorMentions(s *discordgo.Session, m *discordgo.MessageCreate, line string) string {

	// rate limit mentions
	if !config.AllowMention(m.ChannelID) {

		// if mentions in cooldown, make mention bold formated
		matches := moderatorMentions.FindStringSubmatch(line)
		if len(matches) == (1 + 1) {
			mention := matches[1]
			return strings.ReplaceAll(line, mention, fmt.Sprintf("**%s**", mention))
		}

		// don't replace anything
		return line
	}

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
				return strings.ReplaceAll(line, mention, mentionReplace)
			}
			return line
		}
	}
	return line
}

func handleMessageReactions(routineContext context.Context, s *discordgo.Session, msg *discordgo.Message, line, fmtLine string) {

	if strings.Contains(fmtLine, "[kickvote") || strings.Contains(fmtLine, "[specvote") {

		// add reactions to force vote via reactions instead of commands
		errF3 := s.MessageReactionAdd(msg.ChannelID, msg.ID, config.F3Emoji())
		errF4 := s.MessageReactionAdd(msg.ChannelID, msg.ID, config.F4Emoji())
		errBan := s.MessageReactionAdd(msg.ChannelID, msg.ID, config.BanEmoji())

		if errF3 != nil || errF4 != nil || errBan != nil {
			fmtStr := "You have configured an incorrect F3_EMOJI, F4_EMOJI or BAN_EMOJI: \n\t%s\n\t%s\n\t%s\n"
			log.Printf(fmtStr, config.F3Emoji(), config.F4Emoji(), config.BanEmoji())
			config.ResetEmojis()

			s.MessageReactionAdd(msg.ChannelID, msg.ID, config.F3Emoji())
			s.MessageReactionAdd(msg.ChannelID, msg.ID, config.F4Emoji())
			s.MessageReactionAdd(msg.ChannelID, msg.ID, config.BanEmoji())
		}

		matches := formatedSpecVoteKickStringRegex.FindStringSubmatch(fmtLine)
		votingPlayer := Player{ID: -1}
		votedPlayer := Player{ID: -1}

		if len(matches) == 7 {
			votingID, _ := strconv.Atoi(matches[1])
			votedID, _ := strconv.Atoi(matches[3])
			server, ok := config.GetServerByChannelID(msg.ChannelID)
			if !ok {
				return
			}

			votingPlayer = server.Player(votingID)
			votedPlayer = server.Player(votedID)
		}

		// handle votes.
		go voteReactionsRoutine(routineContext, s, msg, votingPlayer, votedPlayer)

	} else if strings.Contains(fmtLine, "**[bans]**") {
		matches := formatedBanRegex.FindStringSubmatch(fmtLine)
		if len(matches) != 4 {
			return
		}

		nickname := matches[1]
		reason := matches[3]

		addr, ok := config.GetAddressByChannelID(msg.ChannelID)
		if !ok {
			return
		}

		server := config.ServerStates[addr]
		playerBan, ok := server.BanServer.GetBanByNameAndReason(nickname, reason)
		if !ok {
			return
		}

		go func(routineContext context.Context, s *discordgo.Session, msg *discordgo.Message, playerBan Ban) {

			defer log.Println("Stopping ban tracking routine of:", playerBan.Player.Name)

			err := s.MessageReactionAdd(msg.ChannelID, msg.ID, config.UnbanEmoji())
			if err != nil {
				fmtStr := "You have configured an incorrect UNBAN_EMOJI:\n\t%s\n"
				log.Printf(fmtStr, config.UnbanEmoji())
				config.ResetEmojis()

				s.MessageReactionAdd(msg.ChannelID, msg.ID, config.UnbanEmoji())
			}

			for {
				select {
				case <-routineContext.Done():
					return
				default:
					if playerBan.Expired() {
						return
					}

					time.Sleep(1 * time.Second)

					unbanUsers, err := s.MessageReactions(msg.ChannelID, msg.ID, config.UnbanEmoji(), 10)
					if err != nil {
						return
					}

					// bot's reaction
					if len(unbanUsers) == 1 {
						continue
					}

					for _, unbanUser := range unbanUsers {
						discordUser := unbanUser.String()

						if config.DiscordModerators.Contains(discordUser) {

							addr, _ := config.ChannelAddress.Get(discordChannel(msg.ChannelID))

							config.DiscordCommandQueue[addr] <- command{
								Author:  discordUser,
								Command: fmt.Sprintf("unban %s", playerBan.Player.IP),
							}
							return
						}

					}
				}
			}

		}(routineContext, s, msg, playerBan)
	}
}

func voteReactionsRoutine(routineContext context.Context, s *discordgo.Session, msg *discordgo.Message, votingPlayer, votedPlayer Player) {

	// a vote takes 30 seconds
	end := time.Now().Add(30 * time.Second)
	for {
		select {
		case <-routineContext.Done():
			return
		default:

			// stopping routine, as the vote timed out.
			if time.Now().After(end) {
				return
			}

			time.Sleep(time.Second)

			f3Users, errF3 := s.MessageReactions(msg.ChannelID, msg.ID, config.F3Emoji(), 10)
			f4Users, errF4 := s.MessageReactions(msg.ChannelID, msg.ID, config.F4Emoji(), 10)
			banUsers, errBan := s.MessageReactions(msg.ChannelID, msg.ID, config.BanEmoji(), 10)
			if errF3 != nil || errF4 != nil || errBan != nil {
				log.Println("Resetting vote emojis to default values, as they could not be retrieved.")
				config.ResetEmojis()

				f3Users, _ = s.MessageReactions(msg.ChannelID, msg.ID, config.F3Emoji(), 10)
				f4Users, _ = s.MessageReactions(msg.ChannelID, msg.ID, config.F4Emoji(), 10)
				banUsers, _ = s.MessageReactions(msg.ChannelID, msg.ID, config.BanEmoji(), 10)
			}

			if len(f3Users) == 1 && len(f4Users) == 1 && len(banUsers) == 1 {
				// the bot's reaction, no user interaction, yet
				continue
			}

			// check for f3 votes
			for _, f3User := range f3Users {

				discordUser := f3User.String()

				if config.DiscordModerators.Contains(discordUser) {

					addr, _ := config.ChannelAddress.Get(discordChannel(msg.ChannelID))

					config.DiscordCommandQueue[addr] <- command{
						Author:  discordUser,
						Command: "vote yes",
					}
					return
				}
			}

			// check f4 votes
			for _, f4User := range f4Users {

				discordUser := f4User.String()

				if config.DiscordModerators.Contains(discordUser) {

					if config.DiscordModerators.Contains(discordUser) {

						addr, _ := config.ChannelAddress.Get(discordChannel(msg.ChannelID))

						config.DiscordCommandQueue[addr] <- command{
							Author:  discordUser,
							Command: "vote no",
						}
						return
					}
				}
			}

			// check ban votes
			for _, banUser := range banUsers {
				if config.DiscordModerators.Contains(banUser.String()) {

					discordUser := banUser.String()

					if config.DiscordModerators.Contains(discordUser) {

						server, _ := config.GetServerByChannelID(msg.ChannelID)
						addr, _ := config.GetAddressByChannelID(msg.ChannelID)

						player := server.PlayerByIP(votingPlayer.IP)
						if player.Valid() {
							// use online player's ID to ban him
							config.DiscordCommandQueue[addr] <- command{
								Author:  discordUser,
								Command: fmt.Sprintf(config.BanReplacementIDCommand, player.ID),
							}

							// abort vote in any case
							config.DiscordCommandQueue[addr] <- command{
								Author:  discordUser,
								Command: "vote no",
							}
						} else {
							// use the IP instead, when the player is not online.
							config.DiscordCommandQueue[addr] <- command{
								Author:  discordUser,
								Command: fmt.Sprintf(config.BanReplacementIPCommand, votingPlayer.IP),
							}

							// abort vote in any case
							config.DiscordCommandQueue[addr] <- command{
								Author:  discordUser,
								Command: "vote no",
							}

							retries := 10
							for {
								select {
								case <-routineContext.Done():
								default:
									retries--
									if retries < 0 {
										return
									}

									ok := server.BanServer.SetPlayerAfterwards(votingPlayer)
									if !ok {
										time.Sleep(time.Second)
										continue
									}

									return
								}
							}
						}

						return
					}
				}
			}

			// continue checking for emote changes
		}
	}
}
