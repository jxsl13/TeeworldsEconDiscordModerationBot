package main

import (
	"fmt"
	"strings"
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
	DiscordModeratorCommands commandSet
	DiscordModeratorRole     string
	DiscordCommandQueue      map[address]chan command
	LogLevel                 int // 0 : chat & votes & rcon,  1: & join & leave,  2: & whisper

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
