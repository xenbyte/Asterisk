#!/usr/bin/env bash
# Creates Kubernetes secrets for Asterisk from environment variables.
# Run once before deploying, or re-run to rotate secrets.
#
# Required env vars:
#   TELEGRAM_BOT_TOKEN   - Telegram bot token from @BotFather
#   ANTHROPIC_API_KEY    - Anthropic API key
#   POSTGRES_PASSWORD    - Password for PostgreSQL
#   ADMIN_TELEGRAM_ID    - Telegram user ID of the bot admin
#
# Usage:
#   export TELEGRAM_BOT_TOKEN=...
#   export ANTHROPIC_API_KEY=...
#   export POSTGRES_PASSWORD=...
#   export ADMIN_TELEGRAM_ID=...
#   ./k8s/create-secrets.sh

set -euo pipefail

: "${TELEGRAM_BOT_TOKEN:?Need TELEGRAM_BOT_TOKEN}"
: "${ANTHROPIC_API_KEY:?Need ANTHROPIC_API_KEY}"
: "${POSTGRES_PASSWORD:?Need POSTGRES_PASSWORD}"
: "${ADMIN_TELEGRAM_ID:?Need ADMIN_TELEGRAM_ID}"

DATABASE_URL="postgres://asterisk:${POSTGRES_PASSWORD}@postgres:5432/asterisk?sslmode=disable"

kubectl create namespace asterisk --dry-run=client -o yaml | kubectl apply -f -

# Postgres secret
kubectl create secret generic postgres-secret \
  --namespace asterisk \
  --from-literal=password="${POSTGRES_PASSWORD}" \
  --dry-run=client -o yaml | kubectl apply -f -

# Bot secret
kubectl create secret generic asterisk-secret \
  --namespace asterisk \
  --from-literal=telegram-bot-token="${TELEGRAM_BOT_TOKEN}" \
  --from-literal=anthropic-api-key="${ANTHROPIC_API_KEY}" \
  --from-literal=database-url="${DATABASE_URL}" \
  --from-literal=admin-telegram-id="${ADMIN_TELEGRAM_ID}" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "✓ Secrets created in namespace 'asterisk'"
