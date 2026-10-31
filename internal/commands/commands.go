// Package commands defines and handles the /bridge slash command tree.
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	"github.com/udihq/OpenPTT/internal/bridge"
)

// Handlers binds slash command routes to the bridge.
type Handlers struct {
	b   *bridge.Bridge
	log *slog.Logger
}

func New(b *bridge.Bridge, log *slog.Logger) *Handlers {
	return &Handlers{b: b, log: log}
}

// Definitions returns the command tree registered per guild.
//
// DefaultMemberPermissions gates these to Manage Server by default. That is
// enforced by Discord, not by us, so there is no permission check to get wrong
// in the handler.
func Definitions() []discord.ApplicationCommandCreate {
	perms := discord.PermissionManageGuild

	userIDOption := func() []discord.ApplicationCommandOption {
		return []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "user_id",
				Description: "Target user ID (right-click user -> Copy User ID). Omit to target yourself.",
				Required:    false,
			},
		}
	}

	return []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			Name:                     "bridge",
			Description:              "Cross-server voice bridge",
			DefaultMemberPermissions: &perms,
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionSubCommand{
					Name:        "start",
					Description: "Join both configured channels and relay audio both ways",
				},
				discord.ApplicationCommandOptionSubCommand{
					Name:        "stop",
					Description: "Stop relaying and leave both channels",
				},
				discord.ApplicationCommandOptionSubCommand{
					Name:        "status",
					Description: "Show bridge state, DAVE readiness, and mute count",
				},
				discord.ApplicationCommandOptionSubCommand{
					Name:        "mute",
					Description: "Mute a user's audio in the bridge",
					Options:     userIDOption(),
				},
				discord.ApplicationCommandOptionSubCommand{
					Name:        "unmute",
					Description: "Unmute a user's audio in the bridge",
					Options:     userIDOption(),
				},
				discord.ApplicationCommandOptionSubCommand{
					Name:        "muteall",
					Description: "Mute every user the bridge has seen this session",
				},
				discord.ApplicationCommandOptionSubCommand{
					Name:        "unmuteall",
					Description: "Clear the entire mute list",
				},
				discord.ApplicationCommandOptionSubCommand{
					Name:        "floor",
					Description: "Toggle floor mode: only one speaker per server is relayed at a time",
				},
			},
		},
	}
}

// Register attaches every route to the router.
func (h *Handlers) Register(r handler.Router) {
	r.SlashCommand("/bridge/start", h.start)
	r.SlashCommand("/bridge/stop", h.stop)
	r.SlashCommand("/bridge/status", h.status)
	r.SlashCommand("/bridge/mute", h.mute)
	r.SlashCommand("/bridge/unmute", h.unmute)
	r.SlashCommand("/bridge/muteall", h.muteAll)
	r.SlashCommand("/bridge/unmuteall", h.unmuteAll)
	r.SlashCommand("/bridge/floor", h.floor)
}

func (h *Handlers) start(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := h.b.Start(ctx); err != nil {
		return respond(e, "❌ "+err.Error(), true)
	}
	return respond(e, "✅ Bridge started. Both channels are joined and audio is relaying in both directions.", false)
}

func (h *Handlers) stop(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	if err := h.b.Stop(context.Background()); err != nil {
		return respond(e, "⚠️ "+err.Error(), true)
	}
	return respond(e, "🛑 Bridge stopped. The bot has left both channels.", false)
}

func (h *Handlers) status(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	s := h.b.Status()

	if !s.Running {
		return respond(e, "**Bridge:** stopped\nUse `/bridge start` to bring it up.", true)
	}

	uptime := time.Since(s.Started).Truncate(time.Second)
	dave := func(ready bool) string {
		if ready {
			return "ready"
		}
		return "**not ready** (no audio will flow)"
	}
	floor := "off"
	if s.Floor {
		floor = "on"
	}

	msg := fmt.Sprintf(
		"**Bridge:** running for `%s`\n"+
			"**A:** <#%d> — DAVE %s\n"+
			"**B:** <#%d> — DAVE %s\n"+
			"**Muted users:** %d\n"+
			"**Floor mode:** %s",
		uptime,
		s.ChannelA, dave(s.DAVEA),
		s.ChannelB, dave(s.DAVEB),
		s.Muted,
		floor,
	)
	return respond(e, msg, true)
}

func (h *Handlers) mute(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	target, err := resolveTarget(data, e)
	if err != nil {
		return respond(e, "❌ "+err.Error(), true)
	}
	h.b.State.Mute(target)
	return respond(e, fmt.Sprintf("🔇 <@%d> is muted in the bridge.", target), false)
}

func (h *Handlers) unmute(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	target, err := resolveTarget(data, e)
	if err != nil {
		return respond(e, "❌ "+err.Error(), true)
	}
	h.b.State.Unmute(target)
	return respond(e, fmt.Sprintf("🔊 <@%d> is unmuted in the bridge.", target), false)
}

func (h *Handlers) muteAll(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	n := h.b.State.MuteAll()
	return respond(e, fmt.Sprintf("🔇 Muted %d user(s). Use `/bridge unmuteall` to clear.", n), false)
}

func (h *Handlers) unmuteAll(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	h.b.State.UnmuteAll()
	return respond(e, "🔊 Mute list cleared.", false)
}

func (h *Handlers) floor(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	next := !h.b.State.Floor()
	h.b.State.SetFloor(next)

	if next {
		return respond(e, "🎙️ Floor mode **on**. One speaker per server at a time; the floor releases after ~400 ms of silence.", false)
	}
	return respond(e, "🎙️ Floor mode **off**. All unmuted speakers are mixed together.", false)
}

// resolveTarget returns the user_id option if supplied, otherwise the invoking
// user. Using a string option rather than a user option keeps the handler
// independent of disgo's resolved-user accessor surface.
func resolveTarget(data discord.SlashCommandInteractionData, e *handler.CommandEvent) (snowflake.ID, error) {
	if raw, ok := data.OptString("user_id"); ok && raw != "" {
		id, err := snowflake.Parse(raw)
		if err != nil {
			return 0, fmt.Errorf("%q is not a valid user ID", raw)
		}
		return id, nil
	}

	id := e.Interaction.User().ID
	if id == 0 {
		return 0, fmt.Errorf("could not determine the invoking user; pass user_id explicitly")
	}
	return id, nil
}

func respond(e *handler.CommandEvent, content string, ephemeral bool) error {
	msg := discord.MessageCreate{Content: content}
	if ephemeral {
		msg.Flags = discord.MessageFlagEphemeral
	}
	_, err := e.CreateMessage(msg)
	return err
}
