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
	serverMessageWidth = 64 - 3
)

// NewAnnouncement creates a new announcement and starts its goroutine
func NewAnnouncement(ctx context.Context, cmdQueue chan<- command, line string) (*Announcement, error) {
	var announcement Announcement

	err := announcement.parse(line)
	if err != nil {
		return nil, err
	}

	announcement.addContext(ctx)
	go announcement.start(cmdQueue)
	return &announcement, nil
}

// Announcement represents an announcement
type Announcement struct {
	ctx     context.Context
	cancel  context.CancelFunc
	Delay   time.Duration
	Message string
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

// addContext adds a context to the server.
func (a *Announcement) addContext(ctx context.Context) {
	if a.ctx != nil && a.cancel != nil {
		a.cancel()
	}

	a.ctx, a.cancel = context.WithCancel(ctx)
}

func sendText(cmdQueue chan<- command, text string) {
	words := strings.Split(text, " ")

	if len(text) == 0 {
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

func (a *Announcement) start(cmdQueue chan<- command) {
	defer log.Printf("closed announcement routine (%s): %s", a.Delay.String(), a.Message)

	ticker := time.Tick(a.Delay)
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker:
			sendText(cmdQueue, a.Message)
		}
	}
}

// Cancel aborts the announcement routine.
func (a *Announcement) Cancel() {
	if a.cancel != nil {
		a.cancel()
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
	return &as
}

// AddAnnouncement adds a new announcement from line.
func (as *AnnouncementServer) AddAnnouncement(line string) error {

	announcement, err := NewAnnouncement(as.ctx, as.commandQueue, line)
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

	copy := Announcement{
		Delay:   original.Delay,
		Message: original.Message,
	}

	return copy, nil
}

// Delete removes a specific announcement from the list
func (as *AnnouncementServer) Delete(index int) (Announcement, error) {
	as.Lock()
	defer as.Unlock()

	if index < 0 || index >= len(as.announcements) {
		return Announcement{}, errors.New("invalid announcement index")
	}

	// cancel announcement routine
	a := as.announcements[index]
	a.Cancel()

	// create copy without cancel & context
	copy := Announcement{
		Delay:   a.Delay,
		Message: a.Message,
	}

	as.announcements = append(as.announcements[:index], as.announcements[index+1:]...)
	return copy, nil
}

// Cancel quits all announcement routines.
func (as *AnnouncementServer) Cancel() {
	as.Lock()
	defer as.Unlock()

	for _, ann := range as.announcements {
		ann.Cancel()
	}

	as.announcements = nil
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
