package bridge

import (
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// Discord voice is 48 kHz stereo Opus in 20 ms frames.
const (
	sampleRate   = 48000
	channels     = 2
	frameSamples = 960                       // samples per channel in 20 ms
	pcmLen       = frameSamples * channels   // interleaved int16 count
	mixInterval  = 20 * time.Millisecond
	staleAfter   = 60 * time.Millisecond // drop a talker who stopped sending
)

// pcmFrame is one decoded 20 ms frame from a single user.
type pcmFrame struct {
	data []int16
	at   time.Time
}

// inbound holds the most recent decoded frame per speaking user for one guild.
//
// The receiver goroutine writes; the mixer goroutine reads. Only the latest
// frame per user is kept: if the mixer falls behind, the older frame is
// superseded rather than queued, which bounds latency instead of letting it
// grow.
type inbound struct {
	mu     sync.Mutex
	frames map[snowflake.ID]pcmFrame
}

func newInbound() *inbound {
	return &inbound{frames: make(map[snowflake.ID]pcmFrame)}
}

func (i *inbound) put(userID snowflake.ID, pcm []int16) {
	if len(pcm) == 0 {
		return
	}
	i.mu.Lock()
	i.frames[userID] = pcmFrame{data: pcm, at: time.Now()}
	i.mu.Unlock()
}

// collect sums every fresh frame into a single 20 ms buffer. Returns nil when
// nobody is talking, which lets the sender emit its own silence frames.
func (i *inbound) collect() []int16 {
	i.mu.Lock()
	defer i.mu.Unlock()

	now := time.Now()
	acc := make([]int32, pcmLen)
	any := false

	for userID, frame := range i.frames {
		if now.Sub(frame.at) > staleAfter {
			delete(i.frames, userID)
			continue
		}
		any = true
		for idx, sample := range frame.data {
			if idx >= pcmLen {
				break
			}
			acc[idx] += int32(sample)
		}
	}

	if !any {
		return nil
	}

	out := make([]int16, pcmLen)
	for idx, v := range acc {
		switch {
		case v > 32767:
			out[idx] = 32767
		case v < -32768:
			out[idx] = -32768
		default:
			out[idx] = int16(v)
		}
	}
	return out
}

func (i *inbound) reset() {
	i.mu.Lock()
	i.frames = make(map[snowflake.ID]pcmFrame)
	i.mu.Unlock()
}
