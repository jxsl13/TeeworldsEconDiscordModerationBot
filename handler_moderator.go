package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// HelpHandler handles the ?help command and prints a help screen
func HelpHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
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

// StatusHandler handles the ?status command
func StatusHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	srv, _ := config.GetServerByChannelID(m.ChannelID)

	// handle status from cache data
	players := srv.Status()

	if len(players) == 0 {
		s.ChannelMessageSend(m.ChannelID, "There are currently no players online.")
		return
	}

	sb := strings.Builder{}
	sb.Grow(64 * len(players))
	for _, p := range players {

		id := WrapInInlineCodeBlock(strconv.Itoa(p.ID))
		version := WrapInInlineCodeBlock(fmt.Sprintf("%x", p.Version))
		name := WrapInInlineCodeBlock(p.Name)
		clan := WrapInInlineCodeBlock(p.Clan)

		line := fmt.Sprintf("%s id=%-4s v=%-5s %-22s %-18s\n", Flag(p.Country), id, version, name, clan)
		sb.WriteString(line)

		if sb.Len() >= 1800 {
			s.ChannelMessageSend(m.ChannelID, sb.String())
			sb.Reset()
		}
	}

	if sb.Len() > 0 {
		s.ChannelMessageSend(m.ChannelID, sb.String())
	}
}

// BansHandler shows the server specific bans list.
func BansHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	srv, ok := config.GetServerByChannelID(m.ChannelID)
	if !ok {
		return
	}

	banSrv := &srv.BanServer

	numBans := banSrv.Size()
	if numBans == 0 {
		s.ChannelMessageSend(m.ChannelID, "[banlist]: 0 ban(s)")
		return
	}
	msg := fmt.Sprintf("[banlist]: %d ban(s)\n```%s```\n", numBans, banSrv.String())
	s.ChannelMessageSend(m.ChannelID, msg)
}

// MultiBanHandler allows to ban a specific player on all moderated servers at once.
func MultiBanHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	server, ok := config.GetServerByChannelID(m.ChannelID)
	if !ok {
		log.Printf("requested multiban from invalid channel by: %s", author)
		return
	}

	id := -1
	minutes := 0
	reason := ""

	cmdTokens := strings.SplitN(args, " ", 3)

	id, err := strconv.Atoi(cmdTokens[0])
	if err != nil || id < 0 {
		s.ChannelMessageSend(m.ChannelID, "**[error]**: invalid user ID")
		return
	}

	minutes, err = strconv.Atoi(cmdTokens[1])
	if err != nil || minutes <= 0 {
		s.ChannelMessageSend(m.ChannelID, "**[error]**: invalid minutes argument, please enter an integer.")
		return
	}

	reason = cmdTokens[2]

	player := server.Player(id)
	ip := player.IP

	cmd := command{
		Author:  m.Author.String(),
		Command: fmt.Sprintf("ban %s %d %s", ip, minutes, reason),
	}

	for _, queue := range config.GetCommandQueues() {
		queue <- cmd
	}

	// set player nickname on all servers
	for _, server := range config.GetServers() {

		for retries := 0; retries < 10; retries++ {
			time.Sleep(time.Second)

			if ok := server.BanServer.SetPlayerAfterwards(player); ok {
				break
			}
		}
	}
}

// MultiUnbanHandler allows to unban a specific IP from all registered servers.
func MultiUnbanHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	server, ok := config.GetServerByChannelID(m.ChannelID)
	if !ok {
		log.Printf("requested multiunban from invalid channel by: %s", m.Author.String())
		return
	}

	id, err := strconv.Atoi(args)
	if err != nil || id < 0 {
		s.ChannelMessageSend(m.ChannelID, "**[error]**: invalid ban ID")
		return
	}

	ban, err := server.BanServer.GetBan(id)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("**[error]**: %s", err.Error()))
		return
	}

	for _, cmdQueue := range config.GetCommandQueues() {
		cmdQueue <- command{
			Author:  author,
			Command: fmt.Sprintf("unban %s", ban.Player.IP),
		}
	}
}

// NotifyHandler registers a notification request that pings the registering moderator when the player joins
func NotifyHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	config.JoinNotify.Add(m.Author.Mention(), args)
	confirmationMessage := fmt.Sprintf("%s's notification request for '%s' received.", m.Author.Mention(), args)
	s.ChannelMessageSend(m.ChannelID, confirmationMessage)
}

// UnnotifyHandler removes all registered notification requests.
func UnnotifyHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	config.JoinNotify.Remove(m.Author.Mention())

	confirmationMessage := fmt.Sprintf("Removed all of %s's notification requests.", m.Author.Mention())
	s.ChannelMessageSend(m.ChannelID, confirmationMessage)
}

// WhoisHandler associates different nicknames to each other based on IPs. This allows
// to check, if a specific player is already known under a different nickname.
func WhoisHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	nickname := strings.TrimSpace(args)
	if config.NicknameTracker == nil {
		s.ChannelMessageSend(m.ChannelID, "nickname tracking is disabled.")
		return
	}

	nicknames, err := config.NicknameTracker.WhoIs(nickname)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, err.Error())
		return
	}

	var sb strings.Builder
	sb.WriteString("**Known nicknames**:\n```\n")
	for _, nick := range nicknames {
		sb.WriteString(nick)
		sb.WriteString("\n")
	}
	sb.WriteString("```\n")
	s.ChannelMessageSend(m.ChannelID, sb.String())
}
