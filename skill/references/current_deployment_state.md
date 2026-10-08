State as of 2026-07-16 deploy attempt:
- Domain: ss.coresynq.cc
- Tunnel id: e2dca43a-bdc6-48d4-9c3b-a1d3e390b9e9
- Tunnel name: ss
- CNAME: ss -> e2dca43a-bdc6-48d4-9c3b-a1d3e390b9e9.cfargotunnel.com
- cloudflared: ~/.local/bin/cloudflared v2026.7.2
- gotty: ~/.local/bin/gotty v1.0.1
- credentials file: ~/.cloudflared/credentials.json present
- systemd user service attempted; first launch failed because no ingress rule was set before start
- Next step required: push tunnel ingress config, enable service, then run gotty-backed session flow from selfservice.sh
