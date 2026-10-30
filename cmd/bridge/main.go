// Command bridge is a two-guild Discord voice relay.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave/golibdave"
	"github.com/joho/godotenv"

	"github.com/udihq/OpenPTT/internal/bridge"
	"github.com/udihq/OpenPTT/internal/commands"
	"github.com/udihq/OpenPTT/internal/config"
)

func main() {
	// .env is optional: in production the values usually come from the
	// service manager or container environment.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	router := handler.New()

	client, err := disgo.New(cfg.Token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				// Guilds populates the guild cache used for command sync.
				gateway.IntentGuilds,
				// VoiceStates is the minimum required for voice joins to work.
				gateway.IntentGuildVoiceStates,
			),
		),
		bot.WithVoiceManagerConfigOpts(
			// Wires DAVE (MLS) via Discord's own libdave. Without this the
			// voice gateway closes the socket right after HELLO.
			voice.WithDaveSessionCreateFunc(golibdave.NewSession),
		),
		bot.WithEventListeners(router),
	)
	if err != nil {
		logger.Error("failed to create discord client", "err", err)
		os.Exit(1)
	}

	br := bridge.New(client, bridge.Config{
		GuildA:   cfg.GuildA,
		ChannelA: cfg.ChannelA,
		GuildB:   cfg.GuildB,
		ChannelB: cfg.ChannelB,
		Bitrate:  cfg.Bitrate,
	}, logger)

	commands.New(br, logger).Register(router)

	// Register slash commands in every guild once we know which guilds we are
	// in. Guild-scoped commands are available immediately, unlike global ones.
	client.AddEventListeners(bot.NewListenerFunc(func(e *events.Ready) {
		defs := commands.Definitions()
		for _, g := range e.Guilds {
			if _, err := client.Rest().SetGuildCommands(client.ApplicationID(), g.ID, defs); err != nil {
				logger.Error("failed to register commands", "guild", g.ID, "err", err)
				continue
			}
			logger.Info("registered commands", "guild", g.ID)
		}
	}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := client.OpenGateway(ctx); err != nil {
		logger.Error("failed to open gateway", "err", err)
		os.Exit(1)
	}

	logger.Info("bridge online",
		"guild_a", cfg.GuildA,
		"guild_b", cfg.GuildB,
		"bitrate", cfg.Bitrate,
	)

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_ = br.Stop(shutdownCtx)
	client.Close(shutdownCtx)
}
