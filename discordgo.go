package main

import "github.com/bwmarrin/discordgo"

// SplitChannelMessageSend properly splits long output in order to accepted by the discord servers.
func SplitChannelMessageSend(s *discordgo.Session, m *discordgo.MessageCreate, text string) error {
	chunks := Split(text, "\n", 1800)

	for _, chunk := range chunks {
		if _, err := s.ChannelMessageSend(m.ChannelID, chunk); err != nil {
			return err
		}
	}

	return nil
}
