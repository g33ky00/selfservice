#!/usr/bin/env bash
set -euo pipefail

export XDG_RUNTIME_DIR=/run/user/$(id -u)
export DBUS_SESSION_BUS_ADDRESS="unix:path=${XDG_RUNTIME_DIR}/bus"

# ── Config ──────────────────────────────────────────────────────────
TTL_SEC="${TTL_SEC:-900}"
CF_CREDS="$HOME/.cloudflared/4a854f51-d2aa-4a42-a026-3e77eb776265.json"
CF_CONFIG="$HOME/.cloudflared/config.yml"
TUNNEL_ID="4a854f51-d2aa-4a42-a026-3e77eb776265"
ACCOUNT_ID="3f84cf68cee5270e8d1032452404564a"
HOST="ss.example.com"
DB="$HOME/.hermes/selfservice/selfservice_sessions.json"
LOCK="$HOME/.hermes/selfservice/.active_session.lock"
GOTTY_BIN="$HOME/.local/bin/gotty"

LOCK_FILE="$HOME/.hermes/selfservice/.active_session.lock"
export ACCOUNT_ID TUNNEL_ID

# ── Helpers ─────────────────────────────────────────────────────────
load_cf_token() {
  if [ ! -f "$CF_CREDS" ]; then
    echo "ERROR: missing $CF_CREDS" >&2
    exit 1
  fi
  CLOUDFLARE_API_TOKEN=$(python3 -c 'import json; print(json.load(open("'$CF_CREDS'"))["AccountTag"])')
  if [ -z "${CLOUDFLARE_API_TOKEN:-}" ]; then
    echo "ERROR: cannot derive token" >&2
    exit 1
  fi
}

rand_token() {
  python3 -c 'import secrets; print(secrets.token_hex(16))'
}

now_iso() {
  date -u +"%Y-%m-%dT%H:%M:%SZ"
}

# ── DB ops ──────────────────────────────────────────────────────────
db_session_add() {
  python3 - <<'PY' "$DB" "$1" "$2" "$3" "$4" "$5" "$(now_iso)" "$TTL_SEC"
import json,sys,datetime,os
p=sys.argv[1]; tok=sys.argv[2]; typ=sys.argv[3]; tgt=sys.argv[4]; port=sys.argv[5]; pid=sys.argv[6]; now=sys.argv[7]; ttl=int(sys.argv[8])
exp=(datetime.datetime.fromisoformat(now.replace('Z','+00:00'))+datetime.timedelta(seconds=ttl)).isoformat()
try:
    with open(p) as f: d=json.load(f)
except Exception:
    d={}
d['sessions']=d.get('sessions',{})
d['sessions'][tok]={'created_at':now,'expires_at':exp,'type':typ,'target':tgt,'port':int(port),'pid':int(pid),'status':'active'}
with open(p,'w') as f: json.dump(d,f,indent=2)
PY
}

db_session_remove() {
  python3 - <<'PY' "$DB" "$1"
import json,sys
p=sys.argv[1]; tok=sys.argv[2]
try:
    with open(p) as f: d=json.load(f)
    s=d.get('sessions',{})
    if tok in s: del s[tok]
    with open(p,'w') as f: json.dump(d,f,indent=2)
except Exception:
    pass
PY
}

db_get_active() {
  python3 - <<'PY' "$DB"
import json,sys
p=sys.argv[1]
try:
    with open(p) as f: d=json.load(f)
    print(json.dumps({k:v for k,v in d.get('sessions',{}).items() if v.get('status')=='active'}))
except Exception:
    print('')
PY
}

# ── CF helpers ──────────────────────────────────────────────────────
_load_cf_env() {
  load_cf_token
  export CLOUDFLARE_API_TOKEN ACCOUNT_ID TUNNEL_ID
}

_yaml_escape() {
  # minimal yaml string escaper for token/path values
  python3 -c 'import sys,json; print(json.dumps(sys.argv[1]))' "$1"
}

cf_reset_ingress() {
  # Write minimal local ingress config and reload daemon (Option B)
  local tmp="$CF_CONFIG.$$.tmp"
  cat > "$tmp" <<EOF
tunnel: $TUNNEL_ID
credentials-file: $CF_CREDS
log-level: info
no-autoupdate: true
ingress:
  - service: http_status:404
EOF
  mv -f "$tmp" "$CF_CONFIG"
  systemctl --user reload cloudflared-ss.service || true
}

cf_set_token_ingress() {
  # Write local ingress config with token rule + catch-all, then reload
  local token="$1"; local port="$2"
  local escaped_token; escaped_token="$(_yaml_escape "/$token")"
  local tmp="$CF_CONFIG.$$.tmp"
  cat > "$tmp" <<EOF
tunnel: $TUNNEL_ID
credentials-file: $CF_CREDS
log-level: info
no-autoupdate: true
ingress:
  - hostname: $HOST
    path: $escaped_token
    service: http://127.0.0.1:$port
  - service: http_status:404
EOF
  mv -f "$tmp" "$CF_CONFIG"
  systemctl --user reload cloudflared-ss.service || true
}

# ── Gotty launcher ──────────────────────────────────────────────────
start_gotty() {
  local target="$1"; local port="$2"; local token="$3"
  mkdir -p "$HOME/.hermes/selfservice/logs"
  local log="$HOME/.hermes/selfservice/logs/gotty_${token}.log"
  "$GOTTY_BIN" -w -r --ws-origin "ss.example.com" --port "$port" --permit-write \
    --address 127.0.0.1 -t 'xterm-256color' \
    "$target" >"$log" 2>&1 &
  local pid=$!
  if [ "$port" -eq 0 ]; then
    sleep 1
    port=$(grep -oE 'Starting server on localhost:[0-9]+' "$log" | head -n1 | grep -oE '[0-9]+' || echo 9000)
  fi
  echo "$pid $port"
}

# ── GC timer ───────────────────────────────────────────────────────
schedule_gc() {
  local token="$1"; local pid="$2"
  local unit="ss-cleanup-${token}"
  local gc_script="$HOME/.local/bin/ss_gc_${token}.sh"
  cat > "$gc_script" <<EOF2
#!/usr/bin/env bash
set -e
echo "[GC] token=$token pid=$pid"
kill "$pid" 2>/dev/null || true
db_session_remove "$token"
cf_reset_ingress || true
rm -f "$gc_script" "$LOCK_FILE"
echo "[GC] done"
EOF2
  chmod +x "$gc_script"
  systemd-run --user --unit="$unit" --on-active="${TTL_SEC}s" "$gc_script" >/dev/null 2>&1 || true
}

# ── Main ────────────────────────────────────────────────────────────
usage() {
  echo "Usage: $0 <ssh|http> <target> [port]" >&2
  exit 1
}
[ $# -lt 2 ] && usage

type="$1"; target="$2"; port="${3:-22}"

echo "=== SelfService session request ==="
echo "type=$type target=$target port=$port"

active=$(db_get_active || true)
if [ -n "${active}" ] && [ "$active" != '{}' ]; then
  echo "ERROR: active session already running" >&2
  exit 1
fi
[ -f "$LOCK_FILE" ] && { echo "ERROR: session in progress" >&2; exit 1; }
touch "$LOCK_FILE"

token=$(rand_token)
echo "session_token=$token"

read -r gotty_pid gotty_port <<< "$(start_gotty "${type}://${target}:${port}" 0 "$token")"
echo "gotty_pid=$gotty_pid port=$gotty_port"

db_session_add "$token" "$type" "$target" "$gotty_port" "$gotty_pid"
cf_set_token_ingress "$token" "$gotty_port"
schedule_gc "$token" "$gotty_pid"

echo "SESSION_LINK=https://${HOST}/${token}"
echo "EXPIRES_IN=${TTL_SEC}s"
