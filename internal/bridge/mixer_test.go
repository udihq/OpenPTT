package bridge

import (
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestInboundCollectSums(t *testing.T) {
	in := newInbound()

	a := make([]int16, pcmLen)
	b := make([]int16, pcmLen)
	for i := range a {
		a[i] = 1000
		b[i] = 2000
	}

	in.put(snowflake.ID(1), a)
	in.put(snowflake.ID(2), b)

	got := in.collect()
	if got == nil {
		t.Fatal("expected a mixed frame, got nil")
	}
	if len(got) != pcmLen {
		t.Fatalf("expected %d samples, got %d", pcmLen, len(got))
	}
	if got[0] != 3000 {
		t.Fatalf("expected 3000, got %d", got[0])
	}
}

func TestInboundCollectClamps(t *testing.T) {
	in := newInbound()

	loud := make([]int16, pcmLen)
	for i := range loud {
		loud[i] = 32767
	}
	in.put(snowflake.ID(1), loud)
	in.put(snowflake.ID(2), loud)

	got := in.collect()
	if got == nil {
		t.Fatal("expected a frame, got nil")
	}
	if got[0] != 32767 {
		t.Fatalf("expected clamping to 32767, got %d", got[0])
	}
}

func TestInboundDropsStaleFrames(t *testing.T) {
	in := newInbound()

	frame := make([]int16, pcmLen)
	in.put(snowflake.ID(1), frame)

	// Backdate the frame past staleAfter.
	in.mu.Lock()
	f := in.frames[snowflake.ID(1)]
	f.at = time.Now().Add(-time.Second)
	in.frames[snowflake.ID(1)] = f
	in.mu.Unlock()

	if got := in.collect(); got != nil {
		t.Fatal("expected stale frame to be dropped")
	}
}

func TestStateMute(t *testing.T) {
	s := NewState()
	user := snowflake.ID(42)

	if !s.Allow(snowflake.ID(1), user) {
		t.Fatal("user should start unmuted")
	}

	s.Mute(user)
	if s.Allow(snowflake.ID(1), user) {
		t.Fatal("muted user should not be allowed")
	}

	s.Unmute(user)
	if !s.Allow(snowflake.ID(1), user) {
		t.Fatal("unmuted user should be allowed again")
	}
}

func TestStateFloorSingleSpeaker(t *testing.T) {
	s := NewState()
	guild := snowflake.ID(1)
	alice := snowflake.ID(100)
	bob := snowflake.ID(200)

	s.SetFloor(true)

	if !s.Allow(guild, alice) {
		t.Fatal("first speaker should take the floor")
	}
	if s.Allow(guild, bob) {
		t.Fatal("second speaker should be blocked while floor is held")
	}
}

func TestStateFloorReleasesAfterSilence(t *testing.T) {
	s := NewState()
	guild := snowflake.ID(1)
	alice := snowflake.ID(100)
	bob := snowflake.ID(200)

	s.SetFloor(true)
	_ = s.Allow(guild, alice)

	// Simulate a long pause by backdating the last accepted frame.
	s.mu.Lock()
	s.lastAt[guild] = time.Now().Add(-time.Second)
	s.mu.Unlock()

	if !s.Allow(guild, bob) {
		t.Fatal("floor should release after silence")
	}
	if s.Allow(guild, alice) {
		t.Fatal("previous holder should lose the floor")
	}
}

func TestStateMuteAll(t *testing.T) {
	s := NewState()
	_ = s.Allow(snowflake.ID(1), snowflake.ID(100))
	_ = s.Allow(snowflake.ID(2), snowflake.ID(200))

	if n := s.MuteAll(); n != 2 {
		t.Fatalf("expected 2 muted users, got %d", n)
	}
	if s.MutedCount() != 2 {
		t.Fatalf("expected MutedCount 2, got %d", s.MutedCount())
	}

	s.UnmuteAll()
	if s.MutedCount() != 0 {
		t.Fatalf("expected 0 after UnmuteAll, got %d", s.MutedCount())
	}
}
