package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

var (
	config = configuration{}

	// context stuff
	globalCtx, globalCancel = context.WithCancel(context.Background())
)

func init() {

	config = configuration{
		EconPasswords:            make(map[Address]password),
		ServerStates:             make(map[Address]*Server),
		ChannelAddress:           newChannelAddressMap(),
		DiscordModerators:        newUserSet(),
		SpiedOnPlayers:           newUserSet(),
		JoinNotify:               newNotifyMap(),
		DiscordModeratorCommands: newCommandSet(),
		DiscordCommandQueue:      make(map[Address]chan command),
		AnnouncemenServers:       make(map[Address]*AnnouncementServer),
		MentionLimiter:           make(map[Address]*RateLimiter),
	}

	env, err := godotenv.Read(".env")
	if err != nil {
		log.Fatal(err)
	}

	discordToken, ok := env["DISCORD_TOKEN"]

	if !ok || discordToken == "" {
		log.Fatal("error: no DISCORD_TOKEN specified")
	}
	config.DiscordToken = discordToken

	discordAdmin, ok := env["DISCORD_ADMIN"]
	if !ok || discordAdmin == "" {
		log.Fatal("error: no DISCORD_ADMIN specified")
	}
	config.DiscordAdmin = discordAdmin
	config.DiscordModerators.Add(discordAdmin)

	econServers, ok := env["ECON_ADDRESSES"]

	if !ok || econServers == "" {
		log.Fatal("error: no ECON_ADDRESSES specified")
	}

	econPasswords, ok := env["ECON_PASSWORDS"]
	if !ok || econPasswords == "" {
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
	config.DiscordModeratorCommands.Add("multiban")
	config.DiscordModeratorCommands.Add("multiunban")
	config.DiscordModeratorCommands.Add("notify")
	config.DiscordModeratorCommands.Add("unnotify")
	config.DiscordModeratorCommands.Add("whois")

	moderatorRole, ok := env["DISCORD_MODERATOR_ROLE"]
	if ok && moderatorRole != "" {
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

	delayString, ok := env["MODERATOR_MENTION_DELAY"]
	if !ok || delayString == "" {
		delayString = "5m"
	}

	mentionDelay, err := time.ParseDuration(delayString)

	if err != nil {
		mentionDelay = 5 * time.Minute
	}

	for idx, addr := range servers {
		config.EconPasswords[Address(addr)] = password(passwords[idx])

		srv := NewServer()
		config.ServerStates[Address(addr)] = srv

		srv.AddJoinHandler(func(p Player) {
			config.NicknameTracker.Add(p)
		})

		config.DiscordCommandQueue[Address(addr)] = make(chan command)
		config.MentionLimiter[Address(addr)] = NewRateLimiter(mentionDelay)
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

	trackNicks := env["NICKNAME_TRACKING"]
	areNicksTracked := false
	switch strings.ToLower(trackNicks) {
	case "", "0", "false", "disable", "disabled":
		areNicksTracked = false
	default:
		areNicksTracked = true
	}

	if areNicksTracked {
		redisAddress, ok := env["REDIS_ADDRESS"]
		if !ok {
			redisAddress = "localhost:6379"
		}

		redisPassword := env["REDIS_PASSWORD"]

		expirationString, ok := env["NICKNAME_EXPIRATION"]
		if !ok {
			expirationString = "120h"
		}

		expirationDelay, err := time.ParseDuration(expirationString)
		if err != nil {
			expirationDelay = 120 * time.Hour
		}

		tracker, err := NewNicknameTracker(redisAddress, redisPassword, expirationDelay)
		if err != nil {
			log.Println(err)
		}

		config.NicknameTracker = tracker

	}

	log.Printf("\n%s", config.String())
}

func main() {
	defer config.Close()

	dg, err := discordgo.New("Bot " + config.DiscordToken)
	if err != nil {
		log.Println(err)
		return
	}

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		// author stays the same
		author := m.Author.String()

		// each new line might contain a command
		lines := strings.Split(m.Content, "\n")

		// try to execute each command
		for _, line := range lines {

			prefix := ""
			command := ""
			args := ""

			if len(line) >= 1 {
				prefix = line[:1]
			} else {
				continue
			}

			if len(line) >= 2 {
				strs := strings.SplitN(line[1:], " ", 2)

				if len(strs) >= 1 {
					command = strs[0]
				}

				if len(strs) == 2 {
					args = strs[1]
				}
			}

			switch prefix {
			case "?":
				if !config.DiscordModerators.Contains(author) {
					s.ChannelMessageSend(m.ChannelID, "no access to moderator commands.")
					continue
				}
				ModeratorCommandsHandler(s, m, author, command, args)
			case "#":
				if author != config.DiscordAdmin {
					s.ChannelMessageSend(m.ChannelID, "no access to admin commands.")
					continue
				}
				AdminCommandsHandler(s, m, author, command, args)
			default:
				continue
			}

		}
	})

	err = dg.Open()
	if err != nil {
		log.Println("error: could not establish a connection to the discord api, please check your credentials")
		return
	}
	defer dg.Close()

	// Wait here until CTRL-C or other term signal is received.
	log.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
	globalCancel()

	log.Println("Shutting down, please wait...")
}
