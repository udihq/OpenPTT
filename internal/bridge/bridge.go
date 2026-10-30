// Package bridge implements a two-guild Discord voice relay.
package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
)

var (
	// ErrRunning is returned by Start when the bridge is already relaying.
	ErrRunning = errors.New("bridge is already running")
	// ErrNotRunning is returned by Stop when there is nothing to stop.
	ErrNotRunning = errors.New("bridge is not running")
)

const defaultBitrate = 64000

// Config identifies the two guilds and the voice channel in each.
type Config struct {
	GuildA   snowflake.ID
	ChannelA snowflake.ID
	GuildB   snowflake.ID
	ChannelB snowflake.ID
	Bitrate  int
}

// Bridge owns the pair of voice endpoints and their lifecycle.
type Bridge struct {
	client  *bot.Client
	cfg     Config
	log     *slog.Logger
	bitrate int

	// State is exported because command handlers need to mutate mute/floor.
	State *State

	mu      sync.Mutex
	a, b    *endpoint
	running bool
	started time.Time
}

func New(client *bot.Client, cfg Config, log *slog.Logger) *Bridge {
	bitrate := cfg.Bitrate
	if bitrate <= 0 {
		bitrate = defaultBitrate
	}
	return &Bridge{
		client:  client,
		cfg:     cfg,
		log:     log,
		bitrate: bitrate,
		State:   NewState(),
	}
}

// Start joins both channels and begins relaying. It is all-or-nothing: if the
// second join fails, the first connection is torn down before returning.
func (b *Bridge) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return ErrRunning
	}
	if b.cfg.GuildA == 0 || b.cfg.GuildB == 0 || b.cfg.ChannelA == 0 || b.cfg.ChannelB == 0 {
		return errors.New("guild and channel IDs must be configured in .env")
	}
	if b.cfg.GuildA == b.cfg.GuildB {
		return errors.New("GUILD_A_ID and GUILD_B_ID must be different servers")
	}

	connA := b.client.VoiceManager().CreateConn(b.cfg.GuildA)
	connB := b.client.VoiceManager().CreateConn(b.cfg.GuildB)

	endA := newEndpoint(b, b.cfg.GuildA, b.cfg.ChannelA, connA)
	endB := newEndpoint(b, b.cfg.GuildB, b.cfg.ChannelB, connB)
	endA.peer, endB.peer = endB, endA

	connA.SetOpusFrameReceiver(endA)
	connA.SetOpusFrameProvider(endA)
	connB.SetOpusFrameReceiver(endB)
	connB.SetOpusFrameProvider(endB)

	if err := openWithTimeout(ctx, connA, b.cfg.ChannelA); err != nil {
		b.teardown(endA, endB)
		return fmt.Errorf("joining guild A (%s): %w", b.cfg.GuildA, err)
	}
	if err := openWithTimeout(ctx, connB, b.cfg.ChannelB); err != nil {
		b.teardown(endA, endB)
		return fmt.Errorf("joining guild B (%s): %w", b.cfg.GuildB, err)
	}

	go endA.run()
	go endB.run()

	b.a, b.b = endA, endB
	b.running = true
	b.started = time.Now()

	b.log.Info("bridge started",
		"guild_a", b.cfg.GuildA, "channel_a", b.cfg.ChannelA,
		"guild_b", b.cfg.GuildB, "channel_b", b.cfg.ChannelB,
	)

	return nil
}

// Stop tears down both connections. Safe to call when not running.
func (b *Bridge) Stop(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return ErrNotRunning
	}

	b.teardown(b.a, b.b)
	b.a, b.b = nil, nil
	b.running = false

	b.log.Info("bridge stopped")
	return nil
}

// teardown closes endpoints and connections. Callers must hold b.mu, except
// Start's failure path which holds it via the deferred unlock.
func (b *Bridge) teardown(endA, endB *endpoint) {
	for _, end := range []*endpoint{endA, endB} {
		if end == nil {
			continue
		}
		end.Close()
	}
	for _, end := range []*endpoint{endA, endB} {
		if end == nil {
			continue
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		end.conn.Close(closeCtx)
		cancel()
		b.client.VoiceManager().RemoveConn(end.guildID)
	}
}

// Status is a snapshot for the /bridge status command.
type Status struct {
	Running  bool
	Started  time.Time
	GuildA   snowflake.ID
	ChannelA snowflake.ID
	GuildB   snowflake.ID
	ChannelB snowflake.ID
	DAVEA    bool
	DAVEB    bool
	Muted    int
	Floor    bool
}

func (b *Bridge) Status() Status {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := Status{
		Running:  b.running,
		Started:  b.started,
		GuildA:   b.cfg.GuildA,
		ChannelA: b.cfg.ChannelA,
		GuildB:   b.cfg.GuildB,
		ChannelB: b.cfg.ChannelB,
		Muted:    b.State.MutedCount(),
		Floor:    b.State.Floor(),
	}

	if b.a != nil {
		s.DAVEA = b.a.conn.DAVE().Ready()
	}
	if b.b != nil {
		s.DAVEB = b.b.conn.DAVE().Ready()
	}

	return s
}

func openWithTimeout(ctx context.Context, conn voice.Conn, channelID snowflake.ID) error {
	openCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// selfMute=false, selfDeaf=false: a deafened bot receives no audio.
	return conn.Open(openCtx, channelID, false, false)
}
