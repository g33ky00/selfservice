# Self-service exposure pattern

## Demand shape
- Default public hostname: `ss.example.com`
- All ephemeral tunnels share the same public hostname; rotation occurs by replacing the active ingress rather than creating a new subdomain per request.
- Request schema:
  - `service`: `ssh`, `http`, `https`, `custom`
  - `port`: internal TCP port
  - `target`: logical name
  - `target_ip`: internal IP
  - `scheme`: `tcp` for generic TCP/SSH, `http`/`https` for web
  - `ttl`: seconds, default 1800, max 7200
  - `email`: OTP recipient for Access

## Scheme/port inference rules
- `22` → `ssh` / `tcp`
- `443` → `https` / `http`
- `8006` → `http` / `http`
- other → `custom` / `http`

## Access policy
- Require authenticated email session via Cloudflare Access OTP.
- Only the requester's email is included; no wildcards.
- Session duration set to `ttl`.

## Cleanup / rotation
- New request replaces the current tunnel ingress for `ss.example.com` rather than creating a new public hostname.
- TTL timer triggers destroy; if user requests a new exposure before TTL expires, destroy the previous session first.
- Keep state in `/tmp/cftunnel_<SESSION_ID>.json` for destroy/logging.

## Operational notes
- `cloudflared` may be installed in `~/.local/bin/cloudflared` if system package install is unavailable.
- systemd `--user` is preferred for runner and timer; `at` is optional.
