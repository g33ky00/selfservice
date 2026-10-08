#!/usr/bin/env bash
# SelfService — Configuration template
# Copy this file to config.local.sh and fill in your own values.
# config.local.sh is gitignored and never committed.

# ── Cloudflare ──────────────────────────────────────────────────────
CF_ACCOUNT_ID="your_account_id"
CF_ZONE_ID="your_zone_id"
CF_API_TOKEN="your_api_token"

# ── Tunnel ──────────────────────────────────────────────────────────
TUNNEL_ID="your_tunnel_uuid"
TUNNEL_CREDENTIALS_FILE="$HOME/.cloudflared/your_tunnel_uuid.json"

# ── Public hostname ─────────────────────────────────────────────────
PUBLIC_HOST="ss.example.com"

# ── Session defaults ────────────────────────────────────────────────
DEFAULT_TTL_SEC=900
MAX_TTL_SEC=7200

# ── Paths ───────────────────────────────────────────────────────────
SESSION_DB_DIR="$HOME/.hermes/selfservice"
SESSION_DB_FILE="$SESSION_DB_DIR/selfservice_sessions.json"
SESSION_LOCK_FILE="$SESSION_DB_DIR/.active_session.lock"
LOG_DIR="$SESSION_DB_DIR/logs"

# ── Gotty ───────────────────────────────────────────────────────────
GOTTY_BIN="$HOME/.local/bin/gotty"

# ── Proxy ───────────────────────────────────────────────────────────
PROXY_PORT=18080
PROXY_LOG="/tmp/ss_proxy.log"
