# Environment Constraints

## Scheduler availability
- `at` is **not installed** on the host running this skill.
- Fallback implemented: **user systemd timer** (`systemctl --user enable --now cf-tunnel-cleanup-<session>.timer`) with `OnActiveSec=<ttl>`.
- Lesson: always detect scheduler at runtime; never hardcode `at` as the only path.

## Script installation path
- `/usr/local/bin` requires root. The skill script must live inside the skill directory and be invoked via absolute path from there.
- Destructive cleanup commands should reference the script via `Path(__file__).resolve()` so the path stays correct if the skill directory moves.

## Tunnel ingress flexibility
- Different targets may need different upstream schemes and TLS verification:
  - `server1`: `https://10.0.0.100:443` + `noTLSVerify: true`
  - `server2`: `http://10.0.0.200:8080` + `noTLSVerify: false`
- The manager must derive `service` from per-target `scheme` rather than hardcoding `https`.

## Cloudflare zone
- Zone ID: `your_zone_id`
- Account ID sourced from `$HOME/.config/cloudflare/credentials.env`

## Network
- Internal services are on a private LAN (RFC 1918 addresses)
- Cloudflare Tunnel provides the public access layer
- No direct inbound connections to internal services

## Paths
- Session DB: `$HOME/.hermes/selfservice/selfservice_sessions.json`
- Session lock: `$HOME/.hermes/selfservice/.active_session.lock`
- Logs: `$HOME/.hermes/selfservice/logs/`
- Gotty binary: `$HOME/.local/bin/gotty`
- Cloudflare config: `$HOME/.cloudflared/config.yml`
