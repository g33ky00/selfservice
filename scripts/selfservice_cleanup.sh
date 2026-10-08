#!/usr/bin/env bash
set -euo pipefail

TOKEN="${1:-}"
PORT="${2:-}"

export XDG_RUNTIME_DIR=/run/user/$(id -u)
export DBUS_SESSION_BUS_ADDRESS="unix:path=${XDG_RUNTIME_DIR}/bus"

cleanup() {
  if [ -n "${PORT:-}" ]; then
    fuser -k -n tcp "${PORT}" 2>/dev/null || true
  fi
  cat > "$HOME/.cloudflared/config.yml" <<'EOF'
tunnel: your_tunnel_uuid
credentials-file: $HOME/.cloudflared/your_tunnel_uuid.json

ingress:
  - service: http_status:404
EOF
  systemctl --user reload cloudflared-ss.service || true
  if [ -n "${TOKEN:-}" ]; then
    python3 - <<PY "$HOME/.hermes/selfservice/selfservice_sessions.json" "$TOKEN"
import json,sys
p,tok=sys.argv[1],sys.argv[2]
try:
    with open(p) as f: d=json.load(f)
    s=d.get('sessions',{})
    if tok in s: del s[tok]
    with open(p,'w') as f: json.dump(d,f,indent=2)
except Exception:
    pass
PY
  fi
}

cleanup
