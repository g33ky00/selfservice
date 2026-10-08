# Current Deployment State

## Active Configuration
- **Public hostname**: `ss.example.com`
- **Tunnel ID**: `your_tunnel_uuid`
- **Cloudflare zone**: `your_zone_id`
- **Session DB**: `$HOME/.hermes/selfservice/selfservice_sessions.json`
- **Default TTL**: 900 seconds (15 minutes)
- **Max TTL**: 7200 seconds (2 hours)

## Services
| Service | Address | Port | Description |
|---|---|---|---|
| server1 | 10.0.0.100 | 443 | Main server (HTTPS) |
| server2 | 10.0.0.200 | 8080 | Secondary server (HTTP) |

## Deployment Checklist
- [x] Cloudflare Tunnel provisioned
- [x] `cloudflared` installed and configured
- [x] `config.local.sh` created with valid credentials
- [x] `selfservice.sh` tested and working
- [x] Session cleanup verified
- [ ] Monitoring/alerting configured
- [ ] Backup strategy for session DB

## Known Issues
- Cloudflare may cache responses — use cache-buster query params if needed
- Background processes may be killed by the shell — use systemd services for production
- URL rewriting may not work for all content types — test with your specific services
