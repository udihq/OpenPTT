# OpenPTT

Free Discord voice bridge with PTT floor mode.

## For end users (after the bot is online)

1. Invite the bot to **both** servers
2. Server 1: `/setup a` → pick voice channel
3. Server 2: `/setup b` → pick voice channel
4. `/bridge start`

No IDs. No config files for users.

### Commands

- `/setup a|b|show` — link channels
- `/bridge start|stop|status`
- `/bridge mute|unmute|muteall|unmuteall`
- `/bridge floor` — one speaker per server

## For the operator (one-time host)

**Required env:** `DISCORD_TOKEN` only.

### Discord app

1. https://discord.com/developers/applications → New → Bot → copy token
2. Invite (replace CLIENT_ID):

```
https://discord.com/oauth2/authorize?client_id=CLIENT_ID&scope=bot+applications.commands&permissions=3146752
```

### Run

Docker (VPS recommended for voice UDP):

```bash
git clone https://github.com/udihq/OpenPTT.git && cd OpenPTT
echo 'DISCORD_TOKEN=your_token' > .env
docker compose up -d
```

Or: `make libdave && make build && DISCORD_TOKEN=... ./bridge`

GitHub hosts **code only**. The bot process needs a VPS/PaaS with outbound UDP (Railway/Render/Fly may work with a volume for `data/`).

## License

MIT
