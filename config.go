package main

import (
	"fmt"
	"strings"
	"sync"
)

type password string
type address string

type command struct {
	Author  string
	Command string
}

type configuration struct {
	EconPasswords            map[address]password
	ServerStates             map[address]*server
	ChannelAddress           channelAddressMap
	DiscordToken             string
	DiscordAdmin             string
	DiscordModerators        userSet
	SpiedOnPlayers           userSet
	DiscordModeratorCommands commandSet
	DiscordModeratorRole     string
	DiscordCommandQueue      map[address]chan command
	LogLevel                 int // 0 : chat & votes & rcon,  1: & whisper, 2: & join & leave

	emojiMu  sync.RWMutex
	f3Emoji  string
	f4Emoji  string
	banEmoji string

	BanReplacementCommand string // formatstring

}

func (c *configuration) ResetEmojis() {
	c.emojiMu.Lock()
	defer c.emojiMu.Unlock()
	c.f3Emoji = "🇾"
	c.f4Emoji = "🇳"
	c.banEmoji = "🔨"
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
	sb.WriteString(fmt.Sprintf("\tF3  : %s\n", c.F3Emoji()))
	sb.WriteString(fmt.Sprintf("\tF4  : %s\n", c.F4Emoji()))
	sb.WriteString(fmt.Sprintf("\tBan : %s\n", c.BanEmoji()))
	sb.WriteString("\n")
	sb.WriteString("Ban Replacement: " + fmt.Sprintf(c.BanReplacementCommand, "{ID}"))
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

	sb.WriteString(fmt.Sprintf("LogLevel: %d\n", c.LogLevel))
	sb.WriteString("\n")

	sb.WriteString("========================================================\n")
	return sb.String()
}
