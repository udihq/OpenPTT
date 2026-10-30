# OpenPTT

Free, self-hosted, FOSS Discord voice bridge with push-to-talk floor control.

Relays live voice audio between two Discord servers (guilds) in both directions, with per-user mute and optional PTT-style floor mode.

```
Server A ──┐                      ┌── Server B
voice chan │  ┌────────────────┐  │  voice chan
users ─────┼─▶│  OpenPTT bridge│◀─┼───── users
           │  │  opus→PCM→mix  │  │
           │  │  PCM→opus      │  │
           │  └────────────────┘  │
           └──────────────────────┘
```

## Why Go + these versions

Discord made **DAVE** mandatory for voice. Bots that skip it get disconnected.

| Component | Version | Note |
|---|---|---|
| disgo | v0.19.3 | Do not upgrade past this without testing channel moves |
| godave / golibdave | v0.3.0 | Official libdave bindings |
| hraban/opus | pinned | Opus encode/decode |

## Features

- Bidirectional voice relay (exactly two guilds)
- Per-user mute
- Optional PTT floor mode
- Mix-minus (no self-echo)
- DAVE-compatible

## Requirements

- Go 1.24+, CGO
- `libopus-dev`, cmake, ninja, build-essential
- Bot with View Channel + Connect + Speak on both voice channels

## Quick start

```bash
cp .env.example .env
# set DISCORD_TOKEN, GUILD_A_ID, CHANNEL_A_ID, GUILD_B_ID, CHANNEL_B_ID

make libdave
make build
./bridge
```

Commands: `/bridge start|stop|status`, mute controls, floor/PTT.

## License

MIT
