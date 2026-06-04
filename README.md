# CF Contest Reminder Discord Bot

A small Go-based Discord bot that posts upcoming Codeforces contest reminders (name, date/time, registration link) to a channel on your Discord server.

## Prerequisites

- Go 1.18+ installed
- A Discord bot application with a bot token and the necessary permissions (Send Messages, Embed Links)
- Developer Mode enabled in Discord to copy channel and server (guild) IDs

## Setup

Create a `.env` file in the project root with the following values:

```
BOT_TOKEN=YOUR_BOT_TOKEN_HERE
DISCORD_ANNOUNCEMENT_CHANNEL_ID=CHANNEL_ID_HERE
DISCORD_GUILD_ID=GUILD_ID_HERE
```

Where to get these values:
- `BOT_TOKEN`: From the Discord Developer Portal → Applications → Your Bot → Token.
- `DISCORD_ANNOUNCEMENT_CHANNEL_ID`: Right-click the channel in Discord and choose "Copy ID" (enable Developer Mode under Appearance → Advanced).
- `DISCORD_GUILD_ID`: Right-click the server icon and choose "Copy ID".

## Run

Start the bot from the project root:

```bash
go run main.go
```

## Files

- `main.go` — application entry point
- `bot/bot.go` — bot implementation
- `sent_contests.json`, `sent_contest_messages.json` — state files used by the bot

## License

This project is licensed under the terms in the `LICENSE` file.