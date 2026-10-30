package bridge

import (
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// floorRelease is how long a guild must be silent before the floor is
// considered free and the next speaker can claim it.
const floorRelease = 400 * time.Millisecond

// State holds the non-audio control state: who is muted, and who currently
// holds the transmit floor in each guild when floor mode is enabled.
//
// All methods are safe for concurrent use. Mutex (not RWMutex) is deliberate:
// every call does a read-modify-write on the floor bookkeeping.
type State struct {
	mu     sync.Mutex
	muted  map[snowflake.ID]struct{}
	seen   map[snowflake.ID]struct{}
	floor  bool
	holder map[snowflake.ID]snowflake.ID // guildID -> userID holding the floor
	lastAt map[snowflake.ID]time.Time    // guildID -> last accepted frame time
}

func NewState() *State {
	return &State{
		muted:  make(map[snowflake.ID]struct{}),
		seen:   make(map[snowflake.ID]struct{}),
		holder: make(map[snowflake.ID]snowflake.ID),
		lastAt: make(map[snowflake.ID]time.Time),
	}
}

// Allow reports whether a frame from userID in guildID should be relayed.
// It also records the user as "seen" so MuteAll has something to act on.
func (s *State) Allow(guildID, userID snowflake.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seen[userID] = struct{}{}

	if _, ok := s.muted[userID]; ok {
		return false
	}
	if !s.floor {
		return true
	}

	now := time.Now()
	if last, ok := s.lastAt[guildID]; !ok || now.Sub(last) > floorRelease {
		s.holder[guildID] = userID
	}
	s.lastAt[guildID] = now

	return s.holder[guildID] == userID
}

func (s *State) Mute(userID snowflake.ID) {
	s.mu.Lock()
	s.muted[userID] = struct{}{}
	s.seen[userID] = struct{}{}
	s.mu.Unlock()
}

func (s *State) Unmute(userID snowflake.ID) {
	s.mu.Lock()
	delete(s.muted, userID)
	s.mu.Unlock()
}

func (s *State) IsMuted(userID snowflake.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.muted[userID]
	return ok
}

// MuteAll mutes every user the bridge has observed this session and reports how
// many were muted.
func (s *State) MuteAll() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for userID := range s.seen {
		s.muted[userID] = struct{}{}
	}
	return len(s.muted)
}

// UnmuteAll clears the mute list entirely.
func (s *State) UnmuteAll() {
	s.mu.Lock()
	s.muted = make(map[snowflake.ID]struct{})
	s.mu.Unlock()
}

func (s *State) MutedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.muted)
}

// SetFloor enables or disables floor mode and resets the holders so the change
// takes effect immediately rather than after the current speaker finishes.
func (s *State) SetFloor(enabled bool) {
	s.mu.Lock()
	s.floor = enabled
	s.holder = make(map[snowflake.ID]snowflake.ID)
	s.lastAt = make(map[snowflake.ID]time.Time)
	s.mu.Unlock()
}

func (s *State) Floor() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.floor
}
