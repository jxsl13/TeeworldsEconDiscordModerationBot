package main

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

// MessageCommandHandler is a function that handles a newly created user message
type MessageCommandHandler func(*discordgo.Session, *discordgo.MessageCreate, string, string, string)

// MessageCommandMiddleware is a wrapper fucntion
type MessageCommandMiddleware func(MessageCommandHandler) MessageCommandHandler

// AdminMessageCreateMiddleware is a wrapper that wraps around specific handler functions in order to deny access to non-admin users.
func AdminMessageCreateMiddleware(next MessageCommandHandler) MessageCommandHandler {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, author, command, args string) {

		if config.DiscordAdmin == "" || m.Author.String() != config.DiscordAdmin {
			s.ChannelMessageSend(m.ChannelID, "you are not allowed to access this command.")
			return
		}
		next(s, m, author, command, args)
	}
}

// ModeratorCommandsHandler handles all moderator commands
func ModeratorCommandsHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, cmd, args string) {
	addr, ok := config.GetAddressByChannelID(m.ChannelID)
	if !ok {
		log.Printf("Request from invalid channel by user %s", author)
		return
	}

	if cmd == "" {
		return
	}

	// check if moderator has access to these commands
	if !config.DiscordModeratorCommands.Contains(cmd) {
		s.ChannelMessageSend(m.ChannelID, "invalid command: "+cmd)
		return
	}

	switch cmd {
	case "help":
		HelpHandler(s, m, author, args)
	case "status":
		StatusHandler(s, m, author, args)
	case "bans":
		BansHandler(s, m, author, args)
	case "multiban":
		MultiBanHandler(s, m, author, args)
	case "multiunban":
		MultiUnbanHandler(s, m, author, args)
	case "notify":
		NotifyHandler(s, m, author, args)
	case "unnotify":
		UnnotifyHandler(s, m, author, args)
	case "whois":
		WhoisHandler(s, m, author, args)
	default:

		// other command sprefixed with ? and that moderators
		//have access to are directly passed to the external console
		config.DiscordCommandQueue[addr] <- command{Author: author, Command: fmt.Sprintf("%s %s", cmd, args)}
	}
}

// AdminCommandsHandler handles the commands of the admin.
func AdminCommandsHandler(s *discordgo.Session, m *discordgo.MessageCreate, author, cmd, args string) {

	addr, ok := config.GetAddressByChannelID(m.ChannelID)
	if !ok && cmd != "moderate" {
		log.Printf("Request from invalid channel by user %s", author)
		return
	}

	if cmd == "" {
		return
	}

	switch cmd {
	case "help":
		HelpHandler(s, m, author, args)
	case "status":
		StatusHandler(s, m, author, args)
	case "bans":
		BansHandler(s, m, author, args)
	case "multiban":
		MultiBanHandler(s, m, author, args)
	case "multiunban":
		MultiUnbanHandler(s, m, author, args)
	case "notify":
		NotifyHandler(s, m, author, args)
	case "unnotify":
		UnnotifyHandler(s, m, author, args)
	case "whois":
		WhoisHandler(s, m, author, args)
	case "ips":
		IPsHandler(s, m, author, args)
	case "announce":
		AnnounceHandler(s, m, author, args)
	case "unannounce":
		UnannounceHandler(s, m, author, args)
	case "announcements":
		AnnouncementsHandler(s, m, author, args)
	case "add":
		AddHandler(s, m, author, args)
	case "remove":
		RemoveHandler(s, m, author, args)
	case "purge":
		PurgeHandler(s, m, author, args)
	case "clean":
		CleanHandler(s, m, author, args)
	case "moderate":
		ModerateHandler(s, m, author, args)
	case "spy":
		SpyHandler(s, m, author, args)
	case "unspy":
		UnspyHandler(s, m, author, args)
	case "purgespy":
		PurgeSpyHandler(s, m, author, args)
	case "execute":
		ExecuteHandler(s, m, author, args)
	case "bulkmultiban":
		BulkMultibanHandler(s, m, author, args)
	default:
		config.DiscordCommandQueue[addr] <- command{Author: author, Command: fmt.Sprintf("%s %s", cmd, args)}
	}
}
