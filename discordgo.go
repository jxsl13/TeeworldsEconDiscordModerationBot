package main

import (
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// SplitChannelMessageSend properly splits long output in order to accepted by the discord servers.
// also properly wrap single codeblocks that were split during this process
func SplitChannelMessageSend(s *discordgo.Session, m *discordgo.MessageCreate, text string) {
	const codeblockDelimiter = "```"

	codeblockFound := strings.Count(text, codeblockDelimiter) == 2
	chunks := Split(text, "\n", 1800)

	codeblockBegin := -1
	codeblockEnd := -1

	if codeblockFound {
		beginSet := false

		for idx, chunk := range chunks {

			delimiterCount := strings.Count(chunk, codeblockDelimiter)
			if delimiterCount == 2 {
				codeblockBegin = idx
				codeblockEnd = idx
				break
			} else if delimiterCount == 1 && !beginSet {
				codeblockBegin = idx
				beginSet = true
			} else if delimiterCount == 1 && beginSet {
				codeblockEnd = idx
				break
			}

		}
	}

	isCodeblockSplit := 0 <= codeblockBegin && codeblockBegin < codeblockEnd

	for idx, chunk := range chunks {

		if isCodeblockSplit {
			if idx == codeblockBegin {
				chunk = chunk + codeblockDelimiter
			} else if codeblockBegin < idx && idx < codeblockEnd {
				chunk = codeblockDelimiter + chunk + codeblockDelimiter
			} else if idx == codeblockEnd {
				chunk = codeblockDelimiter + chunk
			}
		}

		if _, err := s.ChannelMessageSend(m.ChannelID, chunk); err != nil {
			log.Println(err)
		}
	}
}
