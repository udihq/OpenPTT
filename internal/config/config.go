// Package config loads and validates the bridge's runtime configuration from
// environment variables (optionally seeded from a .env file by the caller).
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/disgoorg/snowflake/v2"
)

// Config is the fully validated runtime configuration.
type Config struct {
	Token    string
	GuildA   snowflake.ID
	ChannelA snowflake.ID
	GuildB   snowflake.ID
	ChannelB snowflake.ID
	Bitrate  int
	Debug    bool
}

const defaultBitrate = 64000

// Load reads configuration from the environment and fails fast on anything
// missing or malformed. There are no silent defaults for IDs: a misconfigured
// bridge that half-starts is worse than one that refuses to start.
func Load() (*Config, error) {
	cfg := &Config{
		Token:   os.Getenv("DISCORD_TOKEN"),
		Bitrate: defaultBitrate,
		Debug:   os.Getenv("DEBUG") == "true",
	}

	if cfg.Token == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN is required")
	}

	var err error
	if cfg.GuildA, err = requireID("GUILD_A_ID"); err != nil {
		return nil, err
	}
	if cfg.ChannelA, err = requireID("CHANNEL_A_ID"); err != nil {
		return nil, err
	}
	if cfg.GuildB, err = requireID("GUILD_B_ID"); err != nil {
		return nil, err
	}
	if cfg.ChannelB, err = requireID("CHANNEL_B_ID"); err != nil {
		return nil, err
	}

	if cfg.GuildA == cfg.GuildB {
		return nil, fmt.Errorf("GUILD_A_ID and GUILD_B_ID must be different servers")
	}

	if raw := os.Getenv("BITRATE"); raw != "" {
		bitrate, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("BITRATE: %w", err)
		}
		if bitrate < 6000 || bitrate > 510000 {
			return nil, fmt.Errorf("BITRATE: %d is outside the valid range 6000-510000", bitrate)
		}
		cfg.Bitrate = bitrate
	}

	return cfg, nil
}

func requireID(key string) (snowflake.ID, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	id, err := snowflake.Parse(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a valid snowflake ID", key, raw)
	}
	return id, nil
}
