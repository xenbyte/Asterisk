# Asterisk

Telegram bot for reading classic literature: send photos of book pages, get structured analysis (summary, vocabulary, quotes, cross-page connections) with inline buttons. Sessions persist in BoltDB.

## Prerequisites

- Docker Compose (or Go 1.24+)
- Telegram bot token ([@BotFather](https://t.me/BotFather))
- Your Telegram user ID ([@userinfobot](https://t.me/userinfobot))
- Anthropic API key ([console.anthropic.com](https://console.anthropic.com/))

## Setup

```bash
cp .env.example .env
```

Fill `.env`:

```
TELEGRAM_BOT_TOKEN=...
ANTHROPIC_API_KEY=...
TELEGRAM_ALLOWED_USER_ID=123456789
DATA_DIR=./data
```

## Run

```bash
docker compose up -d
docker compose logs -f
```

Without Docker: `go run ./cmd/bot` (same env vars; use `.env` with godotenv).

## Commands

| Command | Description |
|---------|-------------|
| `/book <title> - <author>` | Set current book |
| `/status` | Current book |
| `/quotes` | All quotes collected for this book |
| `/help` | Help |

Send one or more page photos; use the reply buttons to expand sections.

## Data

Database: `asterisk.db` under `DATA_DIR`. With Compose, volume `bot-data` is mounted at `/data`.

```bash
docker compose cp bot:/data/asterisk.db ./backup.db
```
