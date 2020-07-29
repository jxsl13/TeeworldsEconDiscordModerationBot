package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	serverMessageWidth = 63 - 3
)

// Announcement represents an announcement
type Announcement struct {
	Delay   time.Duration
	Message string
}

// NewAnnouncement creates a new announcement and starts its goroutine
func NewAnnouncement(line string) (*Announcement, error) {
	var announcement Announcement

	err := announcement.parse(line)
	if err != nil {
		return nil, err
	}

	return &announcement, nil
}

// Parse fills the Announcement with
func (a *Announcement) parse(line string) error {
	substrings := strings.SplitN(line, " ", 2)

	if len(substrings) != 2 {
		return errors.New("no space in passed command")
	}

	duration, err := time.ParseDuration(substrings[0])
	if err != nil {
		return errors.New("invalid time format, use the format '3h50m', '3h' or '30m'")
	}

	if duration < time.Minute {
		return errors.New("announcement delay must be at least one minute")
	}

	a.Delay = duration
	a.Message = strings.TrimSpace(substrings[1])

	return nil
}

func sendText(cmdQueue chan<- command, text string) {
	words := strings.Split(text, " ")

	if text == "" {
		return
	}

	buffer := make([]string, 0, len(words))
	bufferStrLen := 0
	for _, word := range words {

		if bufferStrLen+len(buffer)*1+len(word) > serverMessageWidth {

			cmdQueue <- command{
				Author:  "announcement",
				Command: fmt.Sprintf("say %s", strings.TrimSpace(strings.Join(buffer, " "))),
			}
			buffer = buffer[:0]
			bufferStrLen = 0
		}

		buffer = append(buffer, word)
		bufferStrLen += len(word)
	}

	if len(buffer) > 0 {
		cmdQueue <- command{
			Author:  "announcement",
			Command: fmt.Sprintf("say %s", strings.TrimSpace(strings.Join(buffer, " "))),
		}
	}
}

// AnnouncementServer handles per server announcements
type AnnouncementServer struct {
	sync.Mutex
	announcements []*Announcement
	commandQueue  chan<- command
	ctx           context.Context
}

// NewAnnouncementServer creates a new announcement server
func NewAnnouncementServer(ctx context.Context, cmd chan<- command) *AnnouncementServer {
	var as AnnouncementServer
	as.announcements = make([]*Announcement, 0, 1)
	as.commandQueue = cmd
	as.ctx = ctx

	go as.start()

	return &as
}

// Size returns the number of registered announcements
func (as *AnnouncementServer) Size() int {
	as.Lock()
	defer as.Unlock()

	return len(as.announcements)
}

func (as *AnnouncementServer) start() {
	defer log.Println("Closing announcement routine.")

	ticker := time.NewTicker(time.Minute)
	current := -1

	for {
		select {
		case <-as.ctx.Done():
			return
		case <-ticker.C:
			size := as.Size()

			if size == 0 {
				continue
			}
			ticker.Stop()

			current = (current + 1) % size

			ann, err := as.Get(current)
			if err != nil {
				log.Printf("error in announcement routine: %s", err.Error())
				return
			}

			ticker = time.NewTicker(ann.Delay)
			sendText(as.commandQueue, ann.Message)
		}
	}
}

// AddAnnouncement adds a new announcement from line.
func (as *AnnouncementServer) AddAnnouncement(line string) error {

	announcement, err := NewAnnouncement(line)
	if err != nil {
		return err
	}
	as.Lock()
	defer as.Unlock()

	as.announcements = append(as.announcements, announcement)
	return nil
}

// Get returns a copy of the announcement
func (as *AnnouncementServer) Get(index int) (Announcement, error) {
	as.Lock()
	defer as.Unlock()

	if index < 0 || index >= len(as.announcements) {
		return Announcement{}, errors.New("invalid announcement index")
	}

	original := as.announcements[index]

	cpy := Announcement{
		Delay:   original.Delay,
		Message: original.Message,
	}

	return cpy, nil
}

// Delete removes a specific announcement from the list
func (as *AnnouncementServer) Delete(index int) (Announcement, error) {
	as.Lock()
	defer as.Unlock()

	if index < 0 || index >= len(as.announcements) {
		return Announcement{}, errors.New("invalid announcement index")
	}

	a := as.announcements[index]

	// create copy without cancel & context
	cpy := Announcement{
		Delay:   a.Delay,
		Message: a.Message,
	}

	as.announcements = append(as.announcements[:index], as.announcements[index+1:]...)
	return cpy, nil
}

func (as *AnnouncementServer) String() string {

	as.Lock()
	defer as.Unlock()

	var sb strings.Builder

	for idx, ann := range as.announcements {
		sb.WriteString(fmt.Sprintf("ID:**%d** Delay:%6s Message: %-s\n", idx, ann.Delay, ann.Message))
	}

	return sb.String()
}
