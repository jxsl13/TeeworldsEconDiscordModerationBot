package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

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

// IPsHandler allows to check a specific player's knonw IPs. This is helpful if players try to rejoin the server
// after being banned or in any way punished for some reason. These players can then be banned by all their known IPs.
func IPsHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	nickname := strings.TrimSpace(args)
	if config.NicknameTracker == nil {
		s.ChannelMessageSend(m.ChannelID, "nickname tracking is disabled.")
		return
	}

	ips, err := config.NicknameTracker.IPs(nickname)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, err.Error())
		return
	}

	var sb strings.Builder
	sb.WriteString("**Known IPs**:\n```\n")
	for _, ip := range ips {
		sb.WriteString(ip)
		sb.WriteString("\n")
	}
	sb.WriteString("```\n")
	s.ChannelMessageSend(m.ChannelID, sb.String())
}

// AnnounceHandler allows to add a server specific announcement.
func AnnounceHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	as, ok := config.GetAnnouncementServerByChannelID(m.ChannelID)
	if !ok {
		return
	}

	err := as.AddAnnouncement(args)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, err.Error())
		return
	}
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("registered announcement: %s", args))
}

// UnannounceHandler allows to remove an announcement by its id.
func UnannounceHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	index, err := strconv.Atoi(args)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "invalid id argument")
		return
	}

	as, ok := config.GetAnnouncementServerByChannelID(m.ChannelID)
	if !ok {
		s.ChannelMessageSend(m.ChannelID, "invalid channel id")
		return
	}

	ann, err := as.Delete(index)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, err.Error())
		return
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Removed: %s %s", ann.Delay.String(), ann.Message))
}

// AnnouncementsHandler shows a list of registered announcements with their delay and corresponding id.
func AnnouncementsHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	as, ok := config.GetAnnouncementServerByChannelID(m.ChannelID)
	if !ok {
		return
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Announcements:\n%s", as.String()))
}

// AddHandler adds a moderator to the moderators list.
func AddHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	user := strings.Trim(args, " \n")
	config.DiscordModerators.Add(user)
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Added %q to moderators", user))
}

// RemoveHandler removes an admin from the moderators list
func RemoveHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	user := strings.Trim(args, " \n")
	config.DiscordModerators.Remove(user)
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Removed %q from moderators", user))
}

// PurgeHandler removes all moderators except the admin from the moderators list.
func PurgeHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	config.DiscordModerators.Reset()
	config.DiscordModerators.Add(config.DiscordAdmin)
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Purged all moderators except %q", config.DiscordAdmin))
}

// CleanHandler handles cleaning up a channel.
func CleanHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
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

// ModerateHandler starts the connection between the game server and discord.
func ModerateHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	if len(args) == 0 {
		s.ChannelMessageSend(m.ChannelID, "please pass your server econ address.")
		return
	}
	addr := Address(strings.TrimSpace(args))
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
	go serverRoutine(globalCtx, s, m, addr, pass)

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Started listening to server %s", addr))
}

// SpyHandler starts spying on a specific player's whisper messages.
func SpyHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	nickname := strings.Trim(args, " \n")
	config.SpiedOnPlayers.Add(nickname)
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Spying on %q ", nickname))
}

// UnspyHandler stopy the whisper messages spying.
func UnspyHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	nickname := strings.Trim(args, " \n")
	config.SpiedOnPlayers.Remove(nickname)
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Stopped spying on %q", nickname))
}

// PurgeSpyHandler removes all the players from the spied on player list.
func PurgeSpyHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	config.SpiedOnPlayers.Reset()
	s.ChannelMessageSend(m.ChannelID, "Purged all spied on players.")
}

// ExecuteHandler allows to execute any econ command.
func ExecuteHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, args string) {
	// send other messages this way
	addr, ok := config.GetAddressByChannelID(m.ChannelID)
	if !ok {
		return
	}

	config.DiscordCommandQueue[addr] <- command{Author: author, Command: args}
}
