package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/disgoorg/snowflake/v2"
)

// Pair is the two-server voice link.
type Pair struct {
	GuildA   snowflake.ID `json:"guild_a"`
	ChannelA snowflake.ID `json:"channel_a"`
	GuildB   snowflake.ID `json:"guild_b"`
	ChannelB snowflake.ID `json:"channel_b"`
	Bitrate  int          `json:"bitrate,omitempty"`
}

type Store struct {
	mu   sync.Mutex
	path string
	Pair Pair
}

func New(path string) *Store {
	s := &Store{path: path}
	_ = s.Load()
	return s
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.Pair)
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.Pair, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func (s *Store) Get() Pair {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Pair
}

func (s *Store) SetSideA(guild, channel snowflake.ID) error {
	s.mu.Lock()
	s.Pair.GuildA = guild
	s.Pair.ChannelA = channel
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) SetSideB(guild, channel snowflake.ID) error {
	s.mu.Lock()
	s.Pair.GuildB = guild
	s.Pair.ChannelB = channel
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) Ready() bool {
	p := s.Get()
	return p.GuildA != 0 && p.ChannelA != 0 && p.GuildB != 0 && p.ChannelB != 0 && p.GuildA != p.GuildB
}
