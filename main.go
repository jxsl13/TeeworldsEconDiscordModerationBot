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
	config = configuration{}

	startVotekickRegex = regexp.MustCompile(`\[server\]: '([\d]{1,2}):(.*)' voted kick '([\d]{1,2}):(.*)' reason='(.{1,20})' cmd='(.*)' force=([\d])`)
	startSpecVoteRegex = regexp.MustCompile(`\[server\]: '([\d]{1,2}):(.*)' voted spectate '([\d]{1,2}):(.*)' reason='(.{1,20})' cmd='(.*)' force=([\d])`)
	startOptionVote    = regexp.MustCompile(`\[server\]: '([\d]{1,2}):(.*)' voted option '(.+)' reason='(.{1,20})' cmd='(.+)' force=([\d])`)

	// 0: full 1: ID 2: rank
	loginRconRegex     = regexp.MustCompile(`\[server\]: ClientID=(\d+) authed \((.*)\)`)
	executeRconCommand = regexp.MustCompile(`\[server\]: ClientID=([\d]{1,2}) rcon='(.*)'$`)

	chatRegex     = regexp.MustCompile(`\[chat\]: ([\d]+):[\d]+:(.{1,16}): (.*)$`)
	teamChatRegex = regexp.MustCompile(`\[teamchat\]: ([\d]+):[\d]+:(.{1,16}): (.*)$`)
	whisperRegex  = regexp.MustCompile(`\[whisper\]: ([\d]+):[\d]+:(.{1,16}): (.*)$`)

	bansErrorRegex = regexp.MustCompile(`\[net_ban\]: (.*error.*)$`)

	mutesAndVotebansRegex = regexp.MustCompile(`\[Server\]: (.*)`)

	moderatorMentions = regexp.MustCompile(`\[chat\]: .*(@moderators|@mods|@mod|@administrators|@admins|@admin).*`) // first plurals, then singular

	formatedSpecVoteKickStringRegex = regexp.MustCompile(`\*\*\[.*vote.*\]\*\*\: ([\d]+):'(.{0,20})' [^\d]{12,15} ([\d]+):'(.{0,20})'( to spectators)? with reason '(.+)'$`)

	formatedBanRegex = regexp.MustCompile(`\*\*\[bans\]\*\*: '(.*)' banned for (.*) with reason: '(.*)'$`)

	forcedYesRegex = regexp.MustCompile(`\[server\]: forcing vote yes$`)
	forcedNoRegex  = regexp.MustCompile(`\[server\]: forcing vote no$`)

	// context stuff
	globalCtx, globalCancel = context.WithCancel(context.Background())
)

func init() {

	config = configuration{
		EconPasswords:            make(map[address]password),
		ServerStates:             make(map[address]*server),
		ChannelAddress:           newChannelAddressMap(),
		DiscordModerators:        newUserSet(),
		SpiedOnPlayers:           newUserSet(),
		JoinNotify:               newNotifyMap(),
		DiscordModeratorCommands: newCommandSet(),
		DiscordCommandQueue:      make(map[address]chan command),
		AnnouncemenServers:       make(map[address]*AnnouncementServer),
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

	econServers, ok := env["ECON_ADDRESSES"]

	if !ok || len(econServers) == 0 {
		log.Fatal("error: no ECON_ADDRESSES specified")
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
	config.DiscordModeratorCommands.Add("help")
	config.DiscordModeratorCommands.Add("status")
	config.DiscordModeratorCommands.Add("bans")
	config.DiscordModeratorCommands.Add("notify")
	config.DiscordModeratorCommands.Add("unnotify")

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
		log.Fatal("ECON_ADDRESSES and ECON_PASSWORDS mismatch")
	} else if len(servers) == 0 {
		log.Fatal("No ECON_ADDRESSES and/or ECON_PASSWORDS specified.")
	}

	for idx, addr := range servers {
		config.EconPasswords[address(addr)] = password(passwords[idx])

		config.ServerStates[address(addr)] = newServer()

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

	f3emoji, ok := env["F3_EMOJI"]
	if ok && len(f3emoji) > 0 {
		config.SetF3Emoji(f3emoji)
	} else if ok {
		config.SetF3Emoji("🇾")
		log.Println("Expected emoji has the format F3_EMOJI: <:f3:691397485327024209> -> f3:691397485327024209")
	} else {
		config.SetF3Emoji("🇾")
	}

	f4emoji, ok := env["F4_EMOJI"]
	if ok && len(f4emoji) > 0 {
		config.SetF4Emoji(f4emoji)
	} else if ok {
		config.SetF4Emoji("🇳")
		log.Println("Expected emoji has the format F4_EMOJI: <:f4:691397506461859840> -> f4:691397506461859840")
	} else {
		config.SetF4Emoji("🇳")
	}

	banEmoji, ok := env["BAN_EMOJI"]
	if ok && len(banEmoji) > 0 {
		config.SetBanEmoji(banEmoji)
	} else if ok {
		config.SetBanEmoji("🔨")
		log.Println("Expected emoji has the format BAN_EMOJI: <:ban:529812261460508687> -> ban:529812261460508687")
	} else {
		config.SetBanEmoji("🔨")
	}

	banIDReplaceCmd, ok := env["BANID_REPLACEMENT_COMMAND"]
	if ok && (strings.Contains(banIDReplaceCmd, "{ID}") || strings.Contains(banIDReplaceCmd, "{id}")) {
		banIDReplaceCmd = strings.Replace(banIDReplaceCmd, "{ID}", "%d", 1)
		banIDReplaceCmd = strings.Replace(banIDReplaceCmd, "{id}", "%d", 1)
		config.BanReplacementIDCommand = banIDReplaceCmd
	} else {
		config.BanReplacementIDCommand = "ban %d 5 violation of rules"
	}

	banIPReplaceCmd, ok := env["BANIP_REPLACEMENT_COMMAND"]
	if ok && (strings.Contains(banIPReplaceCmd, "{IP}") || strings.Contains(banIPReplaceCmd, "{ip}")) {
		banIPReplaceCmd = strings.Replace(banIPReplaceCmd, "{IP}", "%s", 1)
		banIPReplaceCmd = strings.Replace(banIPReplaceCmd, "{ip}", "%s", 1)
		config.BanReplacementIPCommand = banIPReplaceCmd
	} else {
		config.BanReplacementIDCommand = "ban %d 10 violation of rules"
	}

	unbanEmoji, ok := env["UNBAN_EMOJI"]
	if ok && len(unbanEmoji) > 0 {
		config.SetUnbanEmoji(unbanEmoji)
	} else if ok {
		config.SetUnbanEmoji("❎")
		log.Println("Expected emoji has the format UNBAN_EMOJI: <:sendhelp:529812377441402881> -> sendhelp:529812377441402881")
	} else {
		config.SetUnbanEmoji("❎")
	}

	log.Printf("\n%s", config.String())
}

func parseCommandLine(cmd string) (line string, send bool, err error) {
	args := strings.Split(cmd, " ")
	if len(args) < 1 {
		return
	}
	line = cmd
	send = true
	return
}

func parseEconLine(line string, server *server) (result string, send bool) {

	if strings.Contains(line, "[server]") {
		// contains all commands that contain [server] as prefix.

		// the server consumed the line, so no further
		// parsing is needed
		if consumed, fmtLine := server.ParseLine(line, config.JoinNotify); consumed {
			result = fmtLine
			if fmtLine != "" {
				send = true
			}
			return
		}

		matches := []string{}
		matches = startOptionVote.FindStringSubmatch(line)
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

			result = fmt.Sprintf("**[optionvote%s]**: %d:'%s' voted option '%s' with reason '%s'", forced, votingID, votingName, optionName, reason)
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
			forced := matches[7]

			if forced == "1" {
				forced = "/forced"
			} else {
				forced = ""
			}

			result = fmt.Sprintf("**[kickvote%s]**: %d:'%s' started to kick %d:'%s' with reason '%s'", forced, kickingID, kickingName, kickedID, kickedName, reason)
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
			forced := matches[7]

			if forced == "1" {
				forced = "/forced"
			} else {
				forced = ""
			}

			result = fmt.Sprintf("**[specvote%s]**: %d:'%s' wants to move %d:'%s' to spectators with reason '%s'", forced, votingID, votingName, votedID, votedName, reason)
			send = true
			return
		}

		matches = forcedYesRegex.FindStringSubmatch(line)
		if len(matches) == 1 {
			result = "**[server]**: Forced Yes"
			send = true
			return
		}

		matches = forcedNoRegex.FindStringSubmatch(line)
		if len(matches) == 1 {
			result = "**[server]**: Forced No"
			send = true
			return
		}

		matches = loginRconRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			id, _ := strconv.Atoi(matches[1])
			rank := matches[2]

			result = fmt.Sprintf("**[rcon]**: '%s' authed as **%s**", server.Player(id).Name, rank)
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

		if consumed, fmtLine := server.ParseLine(line, config.JoinNotify); consumed {
			result = fmtLine
			if fmtLine != "" {
				send = true
			}
			return
		}

		matches := bansErrorRegex.FindStringSubmatch(line)
		if len(matches) == (1 + 1) {
			errorMsg := matches[1]
			result = fmt.Sprintf("**[error]**: %s", errorMsg)
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

func handleClean(s *discordgo.Session, m *discordgo.MessageCreate) {
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
}

func handleHelp(s *discordgo.Session, m *discordgo.MessageCreate) {
	// help is not part of the commands
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

	s.ChannelMessageSend(m.ChannelID, sb.String())
}

func handleStatus(s *discordgo.Session, m *discordgo.MessageCreate, addr address) {
	// handle status from cache data
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

	s.ChannelMessageSend(m.ChannelID, sb.String())
}

func handleBans(s *discordgo.Session, m *discordgo.MessageCreate, addr address) {
	numBans := config.ServerStates[addr].BanServer.Size()
	if numBans == 0 {
		s.ChannelMessageSend(m.ChannelID, "[banlist]: 0 ban(s)")
		return
	}
	msg := fmt.Sprintf("[banlist]: %d ban(s)\n```%s```\n", numBans, config.ServerStates[addr].BanServer.String())
	s.ChannelMessageSend(m.ChannelID, msg)
	return
}

func handleNotify(s *discordgo.Session, m *discordgo.MessageCreate, nickname string) {

	config.JoinNotify.Add(m.Author.Mention(), nickname)
	content := fmt.Sprintf("%s's notification request for '%s' received.", m.Author.Mention(), nickname)
	s.ChannelMessageSend(m.ChannelID, content)
}

func handleUnnotify(s *discordgo.Session, m *discordgo.MessageCreate) {
	config.JoinNotify.Remove(m.Author.Mention())

	content := fmt.Sprintf("Removed all of %s's notification requests.", m.Author.Mention())
	s.ChannelMessageSend(m.ChannelID, content)
}

func handleModerate(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	addr := address(args[1])
	pass, ok := config.EconPasswords[addr]

	if !ok {
		s.ChannelMessageSend(m.ChannelID, "unknown server address")
		return
	}

	currentChannel := discordChannel(m.ChannelID)

	// handle single time registration with a discord channel
	if config.ChannelAddress.AlreadyRegistered(addr) {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("The address %s is already registered with a channel.", addr))
		return
	}
	config.ChannelAddress.Set(currentChannel, addr)

	// start routine to listen to specified server.
	go serverRoutine(ctx, s, m, addr, pass)

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Started listening to server %s", addr))
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

func logCleanupRoutine(routineContext context.Context, s *discordgo.Session, channelID, initialMessageID string, addr address) {

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

func commandQueueRoutine(routineContext context.Context, s *discordgo.Session, channelID string, conn *econ.Conn, addr address) {

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
}

func replaceModeratorMentions(s *discordgo.Session, m *discordgo.MessageCreate, line string) string {
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
		votingPlayer := player{ID: -1}
		votedPlayer := player{ID: -1}

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

		addr, ok := config.ChannelAddress.Get(discordChannel(msg.ChannelID))
		if !ok {
			return
		}

		server := config.ServerStates[addr]
		playerBan, ok := server.BanServer.GetBanByNameAndReason(nickname, reason)
		if !ok {
			return
		}

		go func(routineContext context.Context, s *discordgo.Session, msg *discordgo.Message, playerBan ban) {

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
								Command: fmt.Sprintf("unban %s", playerBan.Player.Address),
							}
							return
						}

					}
				}
			}

		}(routineContext, s, msg, playerBan)

	}

}

func voteReactionsRoutine(routineContext context.Context, s *discordgo.Session, msg *discordgo.Message, votingPlayer, votedPlayer player) {

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

						player := server.PlayerByIP(votingPlayer.Address)
						if player.Valid() && player.Online() {
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
								Command: fmt.Sprintf(config.BanReplacementIPCommand, votingPlayer.Address),
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

func serverRoutine(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, addr address, pass password) {
	// channel - server association
	defer config.ChannelAddress.RemoveAddress(addr)
	defer s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Stopped listening to server %s", addr))
	initialMessageID := m.ID

	// sub goroutines
	routineContext, routineCancel := context.WithCancel(ctx)
	defer routineCancel()

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

	for {

		// start routine for waiting for line
		go func() {
			line, err := conn.ReadLine()
			if err != nil {
				log.Println("Closing econ reader routine of:", addr)
				// intended
				return
			}
			result <- line
		}()

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

func main() {
	defer config.Close()

	dg, err := discordgo.New("Bot " + config.DiscordToken)
	if err != nil {
		log.Fatal(err)
	}

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		if strings.HasPrefix(m.Content, "?") {
			if !config.DiscordModerators.Contains(m.Author.String()) {
				return
			}

			addr, ok := config.GetAddressByChannelID(m.ChannelID)
			if !ok {
				log.Printf("Request from invalid channel by user %s", m.Author.String())
				return
			}
			cmd := m.Content[1:]

			if len(cmd) == 0 {
				return
			}

			cmdTokens := strings.Split(cmd, " ")
			if len(cmdTokens) > 0 && !config.DiscordModeratorCommands.Contains(cmdTokens[0]) {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Access to the command %q denied.", cmd))
				return
			}

			switch cmdTokens[0] {
			case "help":
				handleHelp(s, m)
			case "status":
				handleStatus(s, m, addr)
			case "bans":
				handleBans(s, m, addr)
			case "notify":
				argsLine := strings.Join(cmdTokens[1:], " ")
				handleNotify(s, m, strings.TrimSpace(argsLine))
			case "unnotify":
				handleUnnotify(s, m)
			default:
				// send other messages this way
				config.DiscordCommandQueue[addr] <- command{Author: m.Author.String(), Command: cmd}
			}
			return
		}

		if !strings.HasPrefix(m.Content, "#") {
			return
		}

		if m.Author.String() != config.DiscordAdmin {
			response := fmt.Sprintf("Not an admin, only '%s' is allowed to execute commands.", config.DiscordAdmin)
			s.ChannelMessageSend(m.ChannelID, response)
			return
		}

		args := strings.Split(m.Content, " ")

		if len(args) < 1 {
			return
		}

		commandPrefix := args[0][1:]
		arguments := strings.Join(args[1:], " ")

		switch commandPrefix {
		case "announce":
			as, ok := config.GetAnnouncementServerByChannelID(m.ChannelID)
			if !ok {
				break
			}

			err = as.AddAnnouncement(arguments)
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, err.Error())
				break
			}
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("registered announcement: %s", arguments))

		case "unannounce":
			index, err := strconv.Atoi(arguments)
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, "invalid id argument")
				break
			}

			as, ok := config.GetAnnouncementServerByChannelID(m.ChannelID)
			if !ok {
				s.ChannelMessageSend(m.ChannelID, "invalid channel id")
				break
			}

			ann, err := as.Delete(index)
			if err != nil {
				s.ChannelMessageSend(m.ChannelID, err.Error())
				break
			}

			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Removed: %s %s", ann.Delay.String(), ann.Message))

		case "announcements":
			as, ok := config.GetAnnouncementServerByChannelID(m.ChannelID)
			if !ok {
				break
			}

			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Announcements:\n%s", as.String()))

		case "add":

			user := strings.Trim(arguments, " \n")
			config.DiscordModerators.Add(user)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Added %q to moderators", user))
		case "remove":
			user := strings.Trim(arguments, " \n")
			config.DiscordModerators.Remove(user)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Removed %q from moderators", user))
		case "purge":
			config.DiscordModerators.Reset()
			config.DiscordModerators.Add(config.DiscordAdmin)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Purged all moderators except %q", config.DiscordAdmin))
		case "clean":
			handleClean(s, m)
		case "moderate":
			if len(args) < 2 {
				s.ChannelMessageSend(m.ChannelID, "invalid command")
				break
			}
			handleModerate(globalCtx, s, m, args)
		case "spy":
			nickname := strings.Trim(arguments, " \n")
			config.SpiedOnPlayers.Add(nickname)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Spying on %q ", nickname))
		case "unspy":
			nickname := strings.Trim(arguments, " \n")
			config.SpiedOnPlayers.Remove(nickname)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Stopped spying on %q", nickname))
		case "purgespy":
			config.SpiedOnPlayers.Reset()
			s.ChannelMessageSend(m.ChannelID, "Purged all spied on players.")
		case "execute":
			// send other messages this way
			addr, ok := config.GetAddressByChannelID(m.ChannelID)
			if !ok {
				return
			}

			config.DiscordCommandQueue[addr] <- command{Author: m.Author.String(), Command: arguments}
		default:
			s.ChannelMessageSend(m.ChannelID, "invalid command: "+commandPrefix)
		}
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
	globalCancel()

	log.Println("Shutting down, please wait...")
}
