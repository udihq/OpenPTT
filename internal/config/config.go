package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config is runtime config. Only the bot token is required from the environment.
// Guild/channel pairs are set in Discord with /setup and stored on disk.
type Config struct {
	Token    string
	DataPath string
	Bitrate  int
	Debug    bool
}

const defaultBitrate = 64000

func Load() (*Config, error) {
	cfg := &Config{
		Token:    os.Getenv("DISCORD_TOKEN"),
		DataPath: os.Getenv("DATA_PATH"),
		Bitrate:  defaultBitrate,
		Debug:    os.Getenv("DEBUG") == "true",
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN is required — set it in your host env or .env")
	}
	if cfg.DataPath == "" {
		cfg.DataPath = "data/config.json"
	}
	if raw := os.Getenv("BITRATE"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("BITRATE: %w", err)
		}
		if n < 6000 || n > 510000 {
			return nil, fmt.Errorf("BITRATE out of range")
		}
		cfg.Bitrate = n
	}
	return cfg, nil
}
