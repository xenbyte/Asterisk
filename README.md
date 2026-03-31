# Asterisk

Asterisk is a locally deployed Telegram bot that helps you read and understand classic literature. Send photos of book pages from your phone; the bot responds with structured literary analysis via inline buttons and message threading.

The bot has a distinct voice — think of it as a well-read, slightly sardonic companion who has read everything and genuinely cares that you understand what you're reading.

All analysis data is persisted to disk via BoltDB — your book sessions, vocabulary, quotes, and page analyses survive restarts. As you read more pages, the bot builds context and identifies connections between passages.

## Prerequisites

- Docker and Docker Compose (recommended), or Go 1.24+
- A Telegram bot token
- An Anthropic API key

## Setup

### 1. Create a Telegram bot

1. Open Telegram and message [@BotFather](https://t.me/BotFather)
2. Send `/newbot` and follow the prompts
3. Copy the bot token you receive

### 2. Get your Telegram user ID

1. Open Telegram and message [@userinfobot](https://t.me/userinfobot)
2. It will reply with your user ID (a number like `123456789`)

### 3. Get an Anthropic API key

1. Go to [console.anthropic.com](https://console.anthropic.com/)
2. Create an API key

### 4. Configure environment

```bash
cp .env.example .env
```

Edit `.env` and fill in your values:

```
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
ANTHROPIC_API_KEY=sk-ant-...
TELEGRAM_ALLOWED_USER_ID=123456789
DATA_DIR=./data
```

### 5. Run

```bash
docker compose up -d
```

Check logs:

```bash
docker compose logs -f
```

### Run without Docker

```bash
go run ./cmd/bot
```

(Ensure `.env` is present or export the same variables.)

## Publishing to GitHub

The canonical remote is [github.com/xenbyte/Asterisk](https://github.com/xenbyte/Asterisk). The Go module path is `github.com/xenbyte/Asterisk` (matches repo casing).

If you cloned without a remote, or need to add it:

```bash
git remote add origin git@github.com:xenbyte/Asterisk.git
git branch -M main
git push -u origin main
```

If `origin` already exists with a different URL, run `git remote set-url origin git@github.com:xenbyte/Asterisk.git` then push.

Never commit `.env`; it is listed in `.gitignore`.

## Production notes

- **Secrets**: Configure `TELEGRAM_BOT_TOKEN`, `ANTHROPIC_API_KEY`, and `TELEGRAM_ALLOWED_USER_ID` via environment or a secrets manager on the host — not in the image.
- **Access control**: Only the user ID in `TELEGRAM_ALLOWED_USER_ID` can use the bot; keep that aligned with who should have access.
- **Image**: The Dockerfile uses a multi-stage build and runs as a non-root user on distroless — suitable for a small long-running service.
- **Persistence**: Keep the Docker volume (or `DATA_DIR` on disk) backed up; it holds all session data.

## Usage

1. **Set a book**: `/book The Brothers Karamazov - Dostoevsky`
2. **Check current book**: `/status`
3. **Send a photo** of a book page — the bot will analyze it and return a summary with buttons for vocabulary, notable quotes, connections to earlier pages, and things you might have missed
4. **Tap a button** to expand that section
5. **View all quotes**: `/quotes` — see every quote collected from the current book across all analyzed pages

## Commands

| Command | Description |
|---------|-------------|
| `/book <title> - <author>` | Set the book you're currently reading |
| `/status` | Show the currently active book |
| `/quotes` | Show all collected quotes from the current book |
| `/help` | How to use the bot |

## Features

- **Persistent storage** — book sessions, analyses, and quotes survive restarts (BoltDB)
- **Cross-page connections** — the bot remembers previously analyzed pages and identifies recurring themes, callbacks, foreshadowing, and developing arguments
- **Cumulative quote collection** — `/quotes` shows every notable quote found across all pages analyzed for the current book
- **Multi-page analysis** — send multiple photos at once; they're treated as consecutive pages
- **Image quality detection** — blurry or unreadable photos are flagged for retake
- **Graceful shutdown** — clean database closure on SIGINT/SIGTERM

## Data

The bot stores its database as `asterisk.db` under the directory given by `DATA_DIR` (default `./data`). When running with Docker Compose, a named volume `bot-data` is mounted at `/data` inside the container.

To back up your data:

```bash
docker compose cp bot:/data/asterisk.db ./backup.db
```

If you previously used `reading-assistant.db`, rename it to `asterisk.db` in `DATA_DIR` or migrate by copying the file to the new name.
