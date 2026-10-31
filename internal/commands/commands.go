// Package commands defines and handles the /bridge and /setup slash commands.
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
	"github.com/udihq/OpenPTT/internal/store"
)

type Handlers struct {
	b     *bridge.Bridge
	store *store.Store
	log   *slog.Logger
}

func New(b *bridge.Bridge, st *store.Store, log *slog.Logger) *Handlers {
	return &Handlers{b: b, store: st, log: log}
}

func Definitions() []discord.ApplicationCommandCreate {
	perms := discord.PermissionManageGuild

	userIDOption := []discord.ApplicationCommandOption{
		discord.ApplicationCommandOptionString{
			Name:        "user_id",
			Description: "Target user ID (Copy User ID). Omit = yourself.",
			Required:    false,
		},
	}

	return []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			Name:                     "setup",
			Description:              "Link this server voice channel into OpenPTT",
			DefaultMemberPermissions: &perms,
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionSubCommand{
					Name:        "a",
					Description: "Set this server as side A",
					Options: []discord.ApplicationCommandOption{
						discord.ApplicationCommandOptionChannel{
							Name:         "channel",
							Description:  "Voice channel to bridge",
							Required:     true,
							ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildVoice},
						},
					},
				},
				discord.ApplicationCommandOptionSubCommand{
					Name:        "b",
					Description: "Set this server as side B",
					Options: []discord.ApplicationCommandOption{
						discord.ApplicationCommandOptionChannel{
							Name:         "channel",
							Description:  "Voice channel to bridge",
							Required:     true,
							ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildVoice},
						},
					},
				},
				discord.ApplicationCommandOptionSubCommand{
					Name:        "show",
					Description: "Show current bridge pair",
				},
			},
		},
		discord.SlashCommandCreate{
			Name:                     "bridge",
			Description:              "Control the cross-server voice bridge",
			DefaultMemberPermissions: &perms,
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionSubCommand{Name: "start", Description: "Join both channels and relay audio"},
				discord.ApplicationCommandOptionSubCommand{Name: "stop", Description: "Stop and leave both channels"},
				discord.ApplicationCommandOptionSubCommand{Name: "status", Description: "Show state, DAVE, mutes"},
				discord.ApplicationCommandOptionSubCommand{Name: "mute", Description: "Mute a user in the bridge", Options: userIDOption},
				discord.ApplicationCommandOptionSubCommand{Name: "unmute", Description: "Unmute a user", Options: userIDOption},
				discord.ApplicationCommandOptionSubCommand{Name: "muteall", Description: "Mute everyone seen this session"},
				discord.ApplicationCommandOptionSubCommand{Name: "unmuteall", Description: "Clear mute list"},
				discord.ApplicationCommandOptionSubCommand{Name: "floor", Description: "Toggle one-speaker-per-server floor mode"},
			},
		},
	}
}

func (h *Handlers) Register(r handler.Router) {
	r.SlashCommand("/setup/a", h.setupA)
	r.SlashCommand("/setup/b", h.setupB)
	r.SlashCommand("/setup/show", h.setupShow)
	r.SlashCommand("/bridge/start", h.start)
	r.SlashCommand("/bridge/stop", h.stop)
	r.SlashCommand("/bridge/status", h.status)
	r.SlashCommand("/bridge/mute", h.mute)
	r.SlashCommand("/bridge/unmute", h.unmute)
	r.SlashCommand("/bridge/muteall", h.muteAll)
	r.SlashCommand("/bridge/unmuteall", h.unmuteAll)
	r.SlashCommand("/bridge/floor", h.floor)
}

func (h *Handlers) setupA(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	ch, ok := data.OptChannel("channel")
	if !ok {
		return respond(e, "Pick a voice channel.", true)
	}
	var gid snowflake.ID
	if e.GuildID() != nil {
		gid = *e.GuildID()
	}
	if err := h.store.SetSideA(gid, ch.ID); err != nil {
		return respond(e, "Failed to save: "+err.Error(), true)
	}
	h.syncBridge()
	msg := fmt.Sprintf("Side A set to <#%d>.\n", ch.ID)
	if h.store.Ready() {
		msg += "Both sides set. Run /bridge start."
	} else {
		msg += "Now run /setup b in the other server."
	}
	return respond(e, msg, false)
}

func (h *Handlers) setupB(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	ch, ok := data.OptChannel("channel")
	if !ok {
		return respond(e, "Pick a voice channel.", true)
	}
	var gid snowflake.ID
	if e.GuildID() != nil {
		gid = *e.GuildID()
	}
	if err := h.store.SetSideB(gid, ch.ID); err != nil {
		return respond(e, "Failed to save: "+err.Error(), true)
	}
	h.syncBridge()
	msg := fmt.Sprintf("Side B set to <#%d>.\n", ch.ID)
	if h.store.Ready() {
		msg += "Both sides set. Run /bridge start."
	} else {
		msg += "Now run /setup a in the other server."
	}
	return respond(e, msg, false)
}

func (h *Handlers) setupShow(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	p := h.store.Get()
	if !h.store.Ready() {
		return respond(e, "Pair incomplete. /setup a in server 1, /setup b in server 2.", true)
	}
	return respond(e, fmt.Sprintf("A: `%s` -> <#%d>\nB: `%s` -> <#%d>", p.GuildA, p.ChannelA, p.GuildB, p.ChannelB), true)
}

func (h *Handlers) syncBridge() {
	p := h.store.Get()
	h.b.SetPair(p.GuildA, p.ChannelA, p.GuildB, p.ChannelB)
}

func (h *Handlers) start(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	h.syncBridge()
	if !h.store.Ready() {
		return respond(e, "Setup incomplete. /setup a then /setup b.", true)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := h.b.Start(ctx); err != nil {
		return respond(e, err.Error(), true)
	}
	return respond(e, "Bridge live. Audio relays both ways.", false)
}

func (h *Handlers) stop(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	if err := h.b.Stop(context.Background()); err != nil {
		return respond(e, err.Error(), true)
	}
	return respond(e, "Bridge stopped.", false)
}

func (h *Handlers) status(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	s := h.b.Status()
	if !s.Running {
		ready := "incomplete"
		if h.store.Ready() {
			ready = "ready - use /bridge start"
		}
		return respond(e, "Bridge: stopped\nSetup: "+ready, true)
	}
	uptime := time.Since(s.Started).Truncate(time.Second)
	dave := func(r bool) string {
		if r {
			return "ready"
		}
		return "not ready"
	}
	floor := "off"
	if s.Floor {
		floor = "on"
	}
	msg := fmt.Sprintf("Running %s\nA: <#%d> DAVE %s\nB: <#%d> DAVE %s\nMuted: %d\nFloor: %s",
		uptime, s.ChannelA, dave(s.DAVEA), s.ChannelB, dave(s.DAVEB), s.Muted, floor)
	return respond(e, msg, true)
}

func (h *Handlers) mute(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	target, err := resolveTarget(data, e)
	if err != nil {
		return respond(e, err.Error(), true)
	}
	h.b.State.Mute(target)
	return respond(e, fmt.Sprintf("<@%d> muted.", target), false)
}

func (h *Handlers) unmute(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	target, err := resolveTarget(data, e)
	if err != nil {
		return respond(e, err.Error(), true)
	}
	h.b.State.Unmute(target)
	return respond(e, fmt.Sprintf("<@%d> unmuted.", target), false)
}

func (h *Handlers) muteAll(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	n := h.b.State.MuteAll()
	return respond(e, fmt.Sprintf("Muted %d user(s).", n), false)
}

func (h *Handlers) unmuteAll(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	h.b.State.UnmuteAll()
	return respond(e, "Mute list cleared.", false)
}

func (h *Handlers) floor(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	next := !h.b.State.Floor()
	h.b.State.SetFloor(next)
	if next {
		return respond(e, "Floor on - one speaker per server.", false)
	}
	return respond(e, "Floor off - all unmuted mixed.", false)
}

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
		return 0, fmt.Errorf("pass user_id")
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
