package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

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

func handleStatus(s *discordgo.Session, m *discordgo.MessageCreate, addr Address) {
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

func handleBans(s *discordgo.Session, m *discordgo.MessageCreate, addr Address) {
	numBans := config.ServerStates[addr].BanServer.Size()
	if numBans == 0 {
		s.ChannelMessageSend(m.ChannelID, "[banlist]: 0 ban(s)")
		return
	}
	msg := fmt.Sprintf("[banlist]: %d ban(s)\n```%s```\n", numBans, config.ServerStates[addr].BanServer.String())
	s.ChannelMessageSend(m.ChannelID, msg)
	return
}

func handleMultiBan(s *discordgo.Session, m *discordgo.MessageCreate, id, minutes int, reason string) {
	server, ok := config.GetServerByChannelID(m.ChannelID)
	if !ok {
		log.Printf("requested multiban from invalid channel by: %s", m.Author.String())
		return
	}

	player := server.Player(id)
	ip := player.Address

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

func handleMultiUnban(s *discordgo.Session, m *discordgo.MessageCreate, id int) {
	server, ok := config.GetServerByChannelID(m.ChannelID)
	if !ok {
		log.Printf("requested multiunban from invalid channel by: %s", m.Author.String())
		return
	}

	ban, err := server.BanServer.GetBan(id)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("**[error]**: %s", err.Error()))
		return
	}

	for _, cmdQueue := range config.GetCommandQueues() {
		cmdQueue <- command{
			Author:  m.Author.String(),
			Command: fmt.Sprintf("unban %s", ban.Player.Address),
		}
	}
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
	addr := Address(args[1])
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
