package bridge

import (
	"sync"
	"time"

	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/snowflake/v2"
	"github.com/hraban/opus"
)

// endpoint is one side of the bridge: the voice connection for a single guild,
// plus the receive buffer for that guild's speakers and the send queue that
// feeds audio INTO that guild.
//
// Data flow, for endpoints A and B where A.peer == B:
//
//	A.ReceiveOpusFrame  -> decode -> A.in
//	mixer goroutine on B: B.peer.in (== A.in) -> encode -> B.out
//	B.ProvideOpusFrame  -> B.out -> Discord sender for guild B
//
// Because A.out is fed only from B.in, guild A never hears guild A. That is
// mix-minus by construction rather than by filtering, so there is no code path
// that can accidentally echo a server back to itself.
type endpoint struct {
	b         *Bridge
	guildID   snowflake.ID
	channelID snowflake.ID
	conn      voice.Conn
	peer      *endpoint

	in   *inbound
	out  chan []byte
	done chan struct{}
	once sync.Once

	enc *opus.Encoder

	decMu sync.Mutex
	decs  map[snowflake.ID]*opus.Decoder
}

func newEndpoint(b *Bridge, guildID, channelID snowflake.ID, conn voice.Conn) *endpoint {
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err == nil {
		_ = enc.SetBitrate(b.bitrate)
		_ = enc.SetVBR(false)
	} else {
		b.log.Error("failed to create opus encoder", "guild", guildID, "err", err)
	}

	return &endpoint{
		b:         b,
		guildID:   guildID,
		channelID: channelID,
		conn:      conn,
		in:        newInbound(),
		out:       make(chan []byte, 4),
		done:      make(chan struct{}),
		enc:       enc,
		decs:      make(map[snowflake.ID]*opus.Decoder),
	}
}

// run is the mixer goroutine: every 20 ms it takes whatever the peer guild's
// users said, sums it, encodes it, and queues it for this guild's sender.
func (e *endpoint) run() {
	if e.enc == nil {
		return
	}

	ticker := time.NewTicker(mixInterval)
	defer ticker.Stop()

	buf := make([]byte, 4000)

	for {
		select {
		case <-e.done:
			return
		case <-ticker.C:
			if e.peer == nil {
				continue
			}
			pcm := e.peer.in.collect()
			if pcm == nil {
				continue
			}
			n, err := e.enc.Encode(pcm, frameSamples, buf)
			if err != nil || n <= 0 {
				continue
			}
			frame := make([]byte, n)
			copy(frame, buf[:n])
			select {
			case e.out <- frame:
			default:
				// drop if sender is backed up; prefer low latency over completeness
			}
		}
	}
}

// ProvideOpusFrame implements voice.OpusFrameProvider.
func (e *endpoint) ProvideOpusFrame() ([]byte, error) {
	select {
	case <-e.done:
		return nil, nil
	case frame := <-e.out:
		return frame, nil
	default:
		// Returning an empty frame lets disgo's sender emit silence and
		// keeps the connection healthy when the peer is quiet.
		return nil, nil
	}
}

// ReceiveOpusFrame implements voice.OpusFrameReceiver.
func (e *endpoint) ReceiveOpusFrame(userID snowflake.ID, packet *voice.Packet) error {
	if packet == nil || len(packet.Opus) == 0 {
		return nil
	}
	if !e.b.State.Allow(e.guildID, userID) {
		return nil
	}

	dec := e.decoder(userID)
	if dec == nil {
		return nil
	}

	pcm := make([]int16, pcmLen)
	n, err := dec.Decode(packet.Opus, pcm)
	if err != nil {
		// A corrupt packet can leave a decoder in a bad state; drop it and
		// let the next frame allocate a fresh one.
		e.dropDecoder(userID)
		return nil
	}
	if n <= 0 {
		return nil
	}

	samples := n * channels
	if samples > pcmLen {
		samples = pcmLen
	}
	e.in.put(userID, pcm[:samples])

	return nil
}

// CleanupUser implements voice.OpusFrameReceiver. Opus decoders are stateful
// per stream, so a departed user's decoder must not be reused.
func (e *endpoint) CleanupUser(userID snowflake.ID) {
	e.dropDecoder(userID)
}

// Close implements voice.OpusFrameReceiver and is idempotent: disgo may close
// the audio pipeline itself after we have already torn the endpoint down.
func (e *endpoint) Close() {
	e.once.Do(func() {
		close(e.done)

		e.decMu.Lock()
		for id, dec := range e.decs {
			_ = dec.Close()
			delete(e.decs, id)
		}
		e.decMu.Unlock()

		if e.enc != nil {
			_ = e.enc.Close()
			e.enc = nil
		}
	})
}

func (e *endpoint) decoder(userID snowflake.ID) *opus.Decoder {
	e.decMu.Lock()
	defer e.decMu.Unlock()

	if dec, ok := e.decs[userID]; ok {
		return dec
	}

	dec, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		e.b.log.Error("failed to create opus decoder", "user", userID, "err", err)
		return nil
	}
	e.decs[userID] = dec
	return dec
}

func (e *endpoint) dropDecoder(userID snowflake.ID) {
	e.decMu.Lock()
	if dec, ok := e.decs[userID]; ok {
		_ = dec.Close()
		delete(e.decs, userID)
	}
	e.decMu.Unlock()
}
