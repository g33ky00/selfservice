# Environment constraints discovered — 2026-07-03

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
  - `server2`: `http://10.0.0.100:8006` + `noTLSVerify: false`
- The manager must derive `service` from per-target `scheme` rather than hardcoding `https`.

## Cloudflare zone
- `example.com` zone ID: `your_zone_id`
- Account ID sourced from `$HOME/.config/cloudflare/credentials.env`
