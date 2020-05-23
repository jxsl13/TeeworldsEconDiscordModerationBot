package main

import (
	"fmt"
	"strings"
	"sync"
)

type password string

// Address used for ips:port of servers
type Address string

type command struct {
	Author  string
	Command string
}

type configuration struct {
	EconPasswords            map[Address]password
	ServerStates             map[Address]*Server
	ChannelAddress           ChannelAddressMap
	DiscordToken             string
	DiscordAdmin             string
	DiscordModerators        userSet
	SpiedOnPlayers           userSet
	JoinNotify               *NotifyMap
	DiscordModeratorCommands commandSet
	DiscordModeratorRole     string
	MentionLimiter           map[Address]*RateLimiter
	DiscordCommandQueue      map[Address]chan command
	AnnouncemenServers       map[Address]*AnnouncementServer
	LogLevel                 int // 0 : chat & votes & rcon,  1: & whisper, 2: & join & leave

	emojiMu    sync.RWMutex
	f3Emoji    string
	f4Emoji    string
	banEmoji   string
	unbanEmoji string

	BanReplacementIDCommand string // format string
	BanReplacementIPCommand string // format string

	NicknameTracker *NicknameTracker
}

func (c *configuration) GetCommandQueues() []chan command {
	addresses := c.ChannelAddress.GetAddresses()

	cmdQueues := make([]chan command, 0, len(addresses))

	for _, addr := range addresses {
		cmdQueues = append(cmdQueues, c.DiscordCommandQueue[addr])
	}

	return cmdQueues
}

func (c *configuration) GetServers() []*Server {
	addresses := c.ChannelAddress.GetAddresses()

	servers := make([]*Server, 0, len(addresses))

	for _, addr := range addresses {
		servers = append(servers, c.ServerStates[addr])
	}
	return servers
}

func (c *configuration) GetAddressByChannelID(channelID string) (Address, bool) {
	return c.ChannelAddress.Get(discordChannel(channelID))
}

func (c *configuration) GetServerByChannelID(channelID string) (*Server, bool) {
	addr, ok := c.ChannelAddress.Get(discordChannel(channelID))
	if !ok {
		return nil, ok
	}

	server, ok := c.ServerStates[addr]
	if !ok {
		return nil, ok
	}
	return server, true
}

func (c *configuration) GetAnnouncementServerByChannelID(channelID string) (*AnnouncementServer, bool) {
	addr, ok := c.ChannelAddress.Get(discordChannel(channelID))
	if !ok {
		return nil, ok
	}

	as, ok := c.AnnouncemenServers[addr]
	if !ok {
		return nil, ok
	}
	return as, true
}

func (c *configuration) AllowMention(channelID string) (allow bool) {
	addr, ok := c.ChannelAddress.Get(discordChannel(channelID))
	if !ok {
		return false
	}

	ml, ok := c.MentionLimiter[addr]
	if !ok {
		return false
	}

	return ml.Allow()

}

func (c *configuration) ResetEmojis() {
	c.emojiMu.Lock()
	defer c.emojiMu.Unlock()
	c.f3Emoji = "🇾"
	c.f4Emoji = "🇳"
	c.banEmoji = "🔨"
	c.unbanEmoji = "❎"
}

func (c *configuration) F3Emoji() string {
	c.emojiMu.RLock()
	defer c.emojiMu.RUnlock()
	return c.f3Emoji
}

func (c *configuration) F4Emoji() string {
	c.emojiMu.RLock()
	defer c.emojiMu.RUnlock()
	return c.f4Emoji
}

func (c *configuration) BanEmoji() string {
	c.emojiMu.RLock()
	defer c.emojiMu.RUnlock()
	return c.banEmoji
}

func (c *configuration) UnbanEmoji() string {
	c.emojiMu.RLock()
	defer c.emojiMu.RUnlock()
	return c.unbanEmoji
}

func (c *configuration) SetF3Emoji(emoji string) {
	c.emojiMu.Lock()
	defer c.emojiMu.Unlock()
	c.f3Emoji = emoji
}

func (c *configuration) SetF4Emoji(emoji string) {
	c.emojiMu.Lock()
	defer c.emojiMu.Unlock()
	c.f4Emoji = emoji
}

func (c *configuration) SetBanEmoji(emoji string) {
	c.emojiMu.Lock()
	defer c.emojiMu.Unlock()
	c.banEmoji = emoji
}

func (c *configuration) SetUnbanEmoji(emoji string) {
	c.emojiMu.Lock()
	defer c.emojiMu.Unlock()
	c.unbanEmoji = emoji
}

func (c *configuration) Close() {
	for _, c := range c.DiscordCommandQueue {
		close(c)
	}
}

func (c *configuration) String() string {
	sb := strings.Builder{}

	sb.WriteString("==================== Configuration ====================\n")

	sb.WriteString("EconPasswords:\n")
	for addr, pass := range c.EconPasswords {
		sb.WriteString(fmt.Sprintf("\t%s : %s\n", addr, pass))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("DiscordToken : %s\n", c.DiscordToken))
	sb.WriteString("\n")

	sb.WriteString("Discord Emojis:\n")
	sb.WriteString(fmt.Sprintf("\tF3   : %s\n", c.F3Emoji()))
	sb.WriteString(fmt.Sprintf("\tF4   : %s\n", c.F4Emoji()))
	sb.WriteString(fmt.Sprintf("\tBan  : %s\n", c.BanEmoji()))
	sb.WriteString(fmt.Sprintf("\tUnban: %s\n", c.UnbanEmoji()))
	sb.WriteString("\n")

	sb.WriteString("Ban Replacement ID: " + c.BanReplacementIDCommand + "\n")
	sb.WriteString("Ban Replacement IP: " + c.BanReplacementIPCommand + "\n")
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("Administrator: \n\t%s\n\n", c.DiscordAdmin))

	sb.WriteString("Moderators:\n")
	for _, mod := range c.DiscordModerators.Users() {
		sb.WriteString(fmt.Sprintf("\t%s\n", mod))
	}
	sb.WriteString("\n")

	sb.WriteString("Allowed Commands:\n")
	for _, cmd := range c.DiscordModeratorCommands.Commands() {
		sb.WriteString(fmt.Sprintf("\t%s\n", cmd))
	}
	sb.WriteString("\n")

	sb.WriteString("Nickname Tracking: ")
	nickTrack := "enabled"
	if c.NicknameTracker == nil {
		nickTrack = "disabled"
	}
	sb.WriteString(nickTrack)
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("LogLevel: %d\n", c.LogLevel))
	sb.WriteString("\n")

	sb.WriteString("========================================================\n")
	return sb.String()
}
