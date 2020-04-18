package main

import (
	"sync"
	"time"
)

// RateLimiter limits access to 1 access per Delay
type RateLimiter struct {
	delay      time.Duration
	lastAccess time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new Ratelimiter
func NewRateLimiter(delay time.Duration) *RateLimiter {
	return &RateLimiter{
		delay: delay,
	}
}

// Allow returns true if the cooldown expired.
// returns false else.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if time.Since(rl.lastAccess) >= rl.delay {
		rl.lastAccess = time.Now()
		return true
	}
	return false
}
