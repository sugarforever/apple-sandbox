#!/bin/sh
# Registers this container as an OpenAI Agents API self-hosted executor.
#
# Required env (injected by `apple-sandbox run`):
#   CODEX_API_KEY          restricted, environment-scoped key (never the app's main OPENAI_API_KEY)
#   OPENAI_ENVIRONMENT_ID  session.environment.id from the Agents API
#   OPENAI_REMOTE_URL      session.environment.remote_url from the Agents API
#
# NOTE: CODEX_API_KEY as the container env var is corroborated by both
# OpenAI's self-hosted sandboxes guide and Cloudflare's independent reference
# integration (https://developers.cloudflare.com/sandbox/tutorials/openai-agents-api/),
# but has not yet been verified end-to-end against a live Agents API session
# on Apple `container` specifically.
set -eu

: "${CODEX_API_KEY:?CODEX_API_KEY is required}"
: "${OPENAI_ENVIRONMENT_ID:?OPENAI_ENVIRONMENT_ID is required}"
: "${OPENAI_REMOTE_URL:?OPENAI_REMOTE_URL is required}"

exec codex exec-server \
  --remote "$OPENAI_REMOTE_URL" \
  --environment-id "$OPENAI_ENVIRONMENT_ID" \
  "$@"
