# SelfService — Temporary Internal Service Exposure

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Cloudflare](https://img.shields.io/badge/Cloudflare-Tunnel-orange.svg)](https://www.cloudflare.com/products/tunnel/)
[![Python](https://img.shields.io/badge/Python-3.10+-green.svg)](https://www.python.org/)

> Generate a temporary, single-click link to access internal services (NAS dashboard, SSH, web apps) from any browser — without DNS changes or permanent exposure.

---

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [How It Works](#how-it-works)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Usage](#usage)
- [Configuration](#configuration)
- [Security](#security)
- [API Reference](#api-reference)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

**SelfService** is a lightweight system that creates temporary, tokenized URLs to expose internal network services through a Cloudflare Tunnel. Each session generates a unique link that automatically expires after a configurable TTL.

### Features

- **Zero DNS configuration** — Uses a single permanent Cloudflare Tunnel with dynamic ingress rules
- **Token-based access** — Each session has a cryptographically random 32-character token
- **Automatic cleanup** — Sessions self-destruct via systemd timers (default 15 minutes)
- **Multi-protocol** — Supports HTTP/HTTPS web apps and SSH (via Gotty)
- **URL rewriting** — Reverse proxy rewrites relative URLs to work through the tunnel path
- **Single active session** — Prevents accidental exposure of multiple services simultaneously

---

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   User Browser  │────▶│  ss.coresynq.cc  │────▶│  Cloudflare     │
│                 │     │  /<token>        │     │  Tunnel (ss)    │
└─────────────────┘     └──────────────────┘     └────────┬────────┘
                                                          │
                                                          ▼
                                                 ┌─────────────────┐
                                                 │  cloudflared    │
                                                 │  (systemd user) │
                                                 └────────┬────────┘
                                                          │
                                                          ▼
                                                 ┌─────────────────┐
                                                 │  Reverse Proxy  │
                                                 │  (Python :18080)│
                                                 └────────┬────────┘
                                                          │
                                                          ▼
                                                 ┌─────────────────┐
                                                 │  Internal       │
                                                 │  Service        │
                                                 │  (NAS, SSH…)    │
                                                 └─────────────────┘
```

### Components

| Component | Description |
|---|---|
| **Cloudflare Tunnel** | Permanent tunnel (`ss`) with dynamic ingress rules |
| **cloudflared-ss.service** | systemd user service managing the tunnel |
| **Reverse Proxy** | Python HTTP proxy with URL rewriting and cache-busting |
| **Session DB** | JSON file tracking active sessions and metadata |
| **GC Timer** | systemd transient timer for automatic session cleanup |

---

## How It Works

1. **Session Creation** — User requests access to an internal service
2. **Token Generation** — A cryptographically random 32-character token is generated
3. **Ingress Update** — The Cloudflare Tunnel ingress is updated to route `/<token>` to the local proxy
4. **Proxy Launch** — A local reverse proxy starts, forwarding traffic to the internal service
5. **Link Delivery** — User receives `https://ss.coresynq.cc/<token>`
6. **Automatic Cleanup** — After TTL expires, the ingress resets to 404 and the proxy terminates

---

## Prerequisites

### System Requirements

- Linux (Debian/Ubuntu recommended)
- Python 3.10+
- `cloudflared` installed and in `PATH`
- `systemd` with user services support
- `curl` for health checks

### Cloudflare Requirements

- Cloudflare account with Zero Trust access
- A domain managed by Cloudflare (e.g., `coresynq.cc`)
- API token with `Cloudflare Tunnel` and `DNS` read/edit permissions
- A provisioned Cloudflare Tunnel (the script can create one)

### Environment Variables

Create `/home/g33ky/.config/cloudflare/credentials.env`:

```bash
CLOUDFLARE_API_TOKEN=your_api_token_here
CF_ACCOUNT_ID=your_account_id
CF_ZONE_ID=your_zone_id
```

---

## Installation

### 1. Clone the repository

```bash
git clone https://github.com/g33ky00/selfservice.git
cd selfservice
```

### 2. Install cloudflared

```bash
# Debian/Ubuntu
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -o cloudflared.deb
sudo dpkg -i cloudflared.deb

# Or use the install script
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o ~/.local/bin/cloudflared
chmod +x ~/.local/bin/cloudflared
```

### 3. Configure Cloudflare credentials

```bash
mkdir -p ~/.config/cloudflare
cp credentials.env.example ~/.config/cloudflare/credentials.env
chmod 600 ~/.config/cloudflare/credentials.env
# Edit with your actual values
```

### 4. Install the skill (optional)

```bash
cp -r skill/ ~/.hermes/skills/expose_internal_service/
```

---

## Usage

### Create a temporary HTTP link

```bash
./scripts/selfservice.sh http 192.168.2.8 5001
```

**Output:**
```
=== SelfService session request ===
type=http target=192.168.2.8 port=5001
session_token=a1b2c3d4e5f6...
SESSION_LINK=https://ss.coresynq.cc/a1b2c3d4e5f6...
EXPIRES_IN=900s
```

### Create a temporary SSH link (via Gotty)

```bash
./scripts/selfservice.sh ssh 192.168.2.40 22
```

### Custom TTL

```bash
TTL_SEC=1800 ./scripts/selfservice.sh http 192.168.2.8 5001
```

### Using the Hermes skill

Simply ask Hermes:
> "Create a temporary link to http://192.168.2.8:5001"

---

## Configuration

### Script Variables

| Variable | Default | Description |
|---|---|---|
| `TTL_SEC` | `900` | Session lifetime in seconds (max 7200) |
| `HOST` | `ss.coresynq.cc` | Public hostname for the tunnel |
| `TUNNEL_ID` | — | Cloudflare Tunnel UUID |
| `CF_CREDS` | — | Path to tunnel credentials file |
| `DB` | — | Path to session database JSON |

### Cloudflare Tunnel Config

The tunnel ingress is dynamically managed. Default (no active session):

```yaml
tunnel: <TUNNEL_ID>
credentials-file: /path/to/credentials.json
ingress:
  - service: http_status:404
```

Active session:

```yaml
tunnel: <TUNNEL_ID>
credentials-file: /path/to/credentials.json
ingress:
  - hostname: ss.coresynq.cc
    path: /<token>
    service: http://127.0.0.1:18080
  - service: http_status:404
```

---

## Security

### Token Security

- Tokens are generated using `secrets.token_hex(16)` (128 bits of entropy)
- Tokens are never logged or displayed in output
- Session database is stored with `chmod 600`

### Access Control

- **Single active session** — Only one service can be exposed at a time
- **Automatic expiration** — Sessions self-destruct after TTL
- **No authentication** — The token IS the only access control (keep it secret)
- **Ingress isolation** — Expired sessions return 404

### Best Practices

- Use short TTLs (15 minutes default)
- Never share session links in public channels
- Monitor active sessions via the session database
- Use HTTPS-only targets when possible

---

## API Reference

### `selfservice.sh`

```bash
./scripts/selfservice.sh <type> <target> [port]
```

| Parameter | Description |
|---|---|
| `type` | `http` or `ssh` |
| `target` | Internal IP or hostname |
| `port` | Target port (default: 22 for SSH, 80 for HTTP) |

### `cf_tunnel_manager.py`

```bash
python3 skill/scripts/cf_tunnel_manager.py --action <action> [options]
```

| Action | Description |
|---|---|
| `provision` | Create a new session |
| `destroy` | Terminate a session |
| `cleanup` | Remove orphaned session files |

---

## Troubleshooting

### Session link returns 404

- Check if the session has expired: `cat ~/.hermes/selfservice/selfservice_sessions.json`
- Verify the tunnel is active: `systemctl --user status cloudflared-ss.service`
- Check the ingress config: `cat ~/.cloudflared/config.yml`

### Proxy not responding

- Check if the proxy is running: `ss -tlnp | grep 18080`
- Review proxy logs: `journalctl --user -u cloudflared-ss.service`

### URLs not rewritten (blank page)

- This is a known issue with Cloudflare caching
- Try adding a cache-buster query parameter: `?cb=$(date +%s)`
- Consider using a dedicated reverse proxy (nginx, caddy) for production

---

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.

---

## Acknowledgments

- [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/) — for the secure tunnel infrastructure
- [Gotty](https://github.com/sudosu/gotty) — for web-based terminal access
- [Hermes Agent](https://hermes-agent.nousresearch.com/) — for the AI agent integration

---

**Disclaimer**: This tool is provided as-is. Exposing internal services carries security risks. Use at your own discretion and ensure proper network segmentation.
