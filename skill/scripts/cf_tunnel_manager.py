#!/usr/bin/env python3
"""Ephemeral Cloudflare Tunnel Manager — expose_internal_service skill."""

import os
import sys
import json
import time
import base64
import secrets
import subprocess
import urllib.request
import urllib.error
from pathlib import Path

CF_ACCOUNT_ID = os.environ.get("CLOUDFLARE_ACCOUNT_ID")
CF_API_TOKEN = os.environ.get("CLOUDFLARE_API_TOKEN")
CF_ZONE_ID = "67452dcd2d79193536ae440c712f0224"  # coresynq.cc
DEFAULT_EMAIL = "alban.clergeot@proton.me"
CF_BASE = "https://api.cloudflare.com/client/v4"

TARGETS = {
    "pytheas": {
        "host": "pytheas.coresynq.cc",
        "ip": "192.168.2.40",
        "port": "443",
        "scheme": "https",
        "no_tls_verify": True,
    },
    "shiva": {
        "host": "shiva.coresynq.cc",
        "ip": "192.168.2.10",
        "port": "8006",
        "scheme": "http",
        "no_tls_verify": False,
    },
}

STATE_DIR = Path("/tmp")

# Alias interne autorisé
KNOWN_ALIASES = {
    "pytheas": "pytheas",
}


class CfClient:
    def __init__(self):
        if not CF_ACCOUNT_ID or not CF_API_TOKEN:
            raise ValueError("ENV_CLOUDFLARE_MISSING: missing account/token in environment.")
        self.headers = {
            "Authorization": f"Bearer {CF_API_TOKEN}",
            "Content-Type": "application/json",
        }

    def request(self, method, path, payload=None):
        url = f"{CF_BASE}/{path.lstrip('/')}"
        body = json.dumps(payload).encode() if payload is not None else None
        req = urllib.request.Request(url, data=body, method=method, headers=self.headers)
        try:
            with urllib.request.urlopen(req, timeout=20) as resp:
                data = json.loads(resp.read().decode())
        except urllib.error.HTTPError as e:
            detail = e.read().decode()
            return {"ok": False, "http": e.code, "detail": detail}
        if not data.get("success", False):
            return {"ok": False, "http": 200, "detail": data}
        return {"ok": True, "http": 200, "data": data}

    def provision(self, target, ttl, email, session_id):
        t = TARGETS.get(target)
        if not t:
            return None, {"UNKNOWN_TARGET": target}
        hostname = f"{target}-{session_id}.coresynq.cc"
        service = f"{t['scheme']}://{t['ip']}:{t['port']}"
        tunnel_secret = base64.b64encode(secrets.token_bytes(32)).decode()

        r = self.request("POST", f"accounts/{CF_ACCOUNT_ID}/cfd_tunnel", {
            "name": f"ephemeral-{target}-{session_id}",
            "tunnel_secret": tunnel_secret,
        })
        if not r["ok"]:
            return None, {"TUNNEL_CREATE": r.get("detail")}
        tunnel_id = r["data"]["result"]["id"]
        tunnel_token = r["data"]["result"]["token"]

        r = self.request("PUT", f"accounts/{CF_ACCOUNT_ID}/cfd_tunnel/{tunnel_id}/configurations", {
            "config": {
                "ingress": [
                    {
                        "hostname": hostname,
                        "service": service,
                        "originRequest": {"noTLSVerify": t.get("no_tls_verify", False)},
                    },
                    {"service": "http_status:404"},
                ]
            }
        })
        if not r["ok"]:
            self.destroy_tunnel_on_error(tunnel_id)
            return None, {"TUNNEL_CONFIG": r.get("detail")}

        r = self.request("POST", f"zones/{CF_ZONE_ID}/dns_records", {
            "type": "CNAME",
            "name": hostname.split(".")[0],
            "content": f"{tunnel_id}.cfargotunnel.com",
            "proxied": True,
        })
        if not r["ok"]:
            self.destroy_tunnel_on_error(tunnel_id)
            return None, {"DNS_PROVISIONING": r.get("detail")}
        dns_id = r["data"]["result"]["id"]

        r = self.request("POST", f"accounts/{CF_ACCOUNT_ID}/access/apps", {
            "name": f"Access-{target}-{session_id}",
            "domain": hostname,
            "type": "self_hosted",
            "session_duration": f"{int(ttl)}s",
        })
        if not r["ok"]:
            self.destroy_tunnel_on_error(tunnel_id)
            self.destroy_dns_on_error(dns_id)
            return None, {"ACCESS_APP": r.get("detail")}
        app_id = r["data"]["result"]["id"]

        r = self.request("POST", f"accounts/{CF_ACCOUNT_ID}/access/apps/{app_id}/policies", {
            "name": "OTP-Admin-Only",
            "decision": "allow",
            "include": [{"email": {"email": email}}],
        })
        if not r["ok"]:
            self.destroy_app_on_error(app_id)
            self.destroy_tunnel_on_error(tunnel_id)
            self.destroy_dns_on_error(dns_id)
            return None, {"ACCESS_POLICY": r.get("detail")}

        state = {
            "tunnel_id": tunnel_id,
            "dns_id": dns_id,
            "app_id": app_id,
            "session_id": session_id,
            "target": target,
            "ttl": int(ttl),
            "created": int(time.time()),
        }
        state_path = STATE_DIR / f"cftunnel_{session_id}.json"
        state_path.write_text(json.dumps(state))
        return state, tunnel_secret, tunnel_token

    def destroy_tunnel_on_error(self, tunnel_id):
        if tunnel_id:
            self.request("DELETE", f"accounts/{CF_ACCOUNT_ID}/cfd_tunnel/{tunnel_id}")

    def destroy_dns_on_error(self, dns_id):
        if dns_id:
            self.request("DELETE", f"zones/{CF_ZONE_ID}/dns_records/{dns_id}")

    def destroy_app_on_error(self, app_id):
        if app_id:
            self.request("DELETE", f"accounts/{CF_ACCOUNT_ID}/access/apps/{app_id}")

    def destroy(self, session_id):
        p = STATE_DIR / f"cftunnel_{session_id}.json"
        if not p.exists():
            return {"status": "destroyed"}
        s = json.loads(p.read_text())
        ops = []
        for kind, rid in (
            ("ACCESS_APP", s.get("app_id")),
            ("DNS", s.get("dns_id")),
            ("TUNNEL", s.get("tunnel_id")),
        ):
            if not rid:
                continue
            path_map = {
                "ACCESS_APP": f"accounts/{CF_ACCOUNT_ID}/access/apps/{rid}",
                "DNS": f"zones/{CF_ZONE_ID}/dns_records/{rid}",
                "TUNNEL": f"accounts/{CF_ACCOUNT_ID}/cfd_tunnel/{rid}",
            }
            r = self.request("DELETE", path_map[kind])
            ops.append((kind, bool(r.get("ok", False)), r.get("detail")))
        p.unlink(missing_ok=True)
        return {"status": "destroyed", "ops": ops}

    def cleanup(self, max_age=3600):
        deleted = 0
        for p in STATE_DIR.glob("cftunnel_*.json"):
            try:
                s = json.loads(p.read_text())
            except Exception:
                p.unlink(missing_ok=True)
                deleted += 1
                continue
            age = time.time() - s.get("created", 0)
            if age >= max_age:
                self.destroy(s.get("session_id"))
                deleted += 1
        return {"status": "ok", "count": deleted}


def run_local(session_id, ttl, token):
    unit = f"cf-tunnel-{session_id}"
    cmd = [
        "systemd-run",
        "--user",
        "--quiet",
        f"--unit={unit}",
        f"--property=RuntimeMaxSec={int(ttl)}",
        "cloudflared",
        "tunnel",
        "run",
        "--token",
        token,
    ]
    p = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    if p.returncode != 0:
        return False, p.stderr.strip() or p.stdout.strip()
    return True, unit


def schedule_destroy(session_id, ttl):
    script_path = Path(__file__).with_name("cf_tunnel_manager.py").resolve()
    cmd = f'python3 {script_path} --action destroy --session-id {session_id}'
    if shutil_which("at"):
        p = subprocess.run(["at", "now", "+", f"{int(ttl)}", "seconds"], input=cmd, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        return p.returncode == 0, p.stderr.strip()
    svc = (
        "[Unit]\n"
        f"Description=CF tunnel cleanup {session_id}\n"
        "[Service]\n"
        "Type=oneshot\n"
        f"ExecStart=/usr/bin/python3 /home/g33ky/.hermes/profiles/dev/skills/devops/expose_internal_service/scripts/cf_tunnel_manager.py --action destroy --session-id {session_id}\n"
    )
    timer = (
        "[Unit]\n"
        f"Description=CF tunnel cleanup timer {session_id}\n"
        "[Timer]\n"
        f"OnActiveSec={int(ttl)}\n"
        f"Unit=cf-tunnel-cleanup-{session_id}.service\n"
        "[Install]\n"
        "WantedBy=timers.target\n"
    )
    Path(f"/tmp/cf-tunnel-cleanup-{session_id}.service").write_text(svc)
    Path(f"/tmp/cf-tunnel-cleanup-{session_id}.timer").write_text(timer)
    p = subprocess.run(["systemctl", "--user", "enable", "--now", f"cf-tunnel-cleanup-{session_id}.timer"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    return p.returncode == 0, p.stderr.strip()


def shutil_which(cmd):
    return any(
        os.access(os.path.join(p, cmd), os.X_OK)
        for p in os.environ.get("PATH", os.defpath).split(os.pathsep)
    )


def main():
    action = None
    session_id = None
    target = None
    ttl = 1800
    email = None
    args = sys.argv[1:]
    for i in range(len(args)):
        if args[i] == "--action" and i + 1 < len(args):
            action = args[i + 1]
        elif args[i] == "--session-id" and i + 1 < len(args):
            session_id = args[i + 1]
        elif args[i] == "--target" and i + 1 < len(args):
            target = args[i + 1]
        elif args[i] == "--ttl" and i + 1 < len(args):
            ttl = int(args[i + 1])
        elif args[i] == "--email" and i + 1 < len(args):
            email = args[i + 1]

    if action == "destroy":
        if not session_id:
            print(json.dumps({"status": "error", "step_failed": "ARGS", "details": "session-id required"}), flush=True)
            return
        print(json.dumps(CfClient().destroy(session_id)), flush=True)
        return

    if action == "cleanup":
        print(json.dumps(CfClient().cleanup()), flush=True)
        return

    if not target:
        print(json.dumps({"status": "error", "step_failed": "ARGS", "details": "target required"}), flush=True)
        return
    if not email:
        email = DEFAULT_EMAIL

    if ttl < 1:
        ttl = 60
    elif ttl > 7200:
        ttl = 7200

    session_id = f"{int(time.time())%1000000000:09x}{secrets.token_hex(2)}"
    CfClient().cleanup(max_age=3600)
    client = CfClient()
    res = client.provision(target, ttl, email, session_id)
    if not isinstance(res, tuple) or len(res) != 3:
        if isinstance(res, tuple) and len(res) == 2:
            print(json.dumps(res[1]), flush=True)
        else:
            print(json.dumps(res), flush=True)
        return
    state, tunnel_secret, tunnel_token = res
    hostname = f"{target}-{session_id}.coresynq.cc"
    ok, info = run_local(session_id, ttl, tunnel_token)
    if not ok:
        print(json.dumps({"status": "error", "step_failed": "LOCAL_RUNNER", "details": info}), flush=True)
        return
    sched_ok, sched_err = schedule_destroy(session_id, ttl)
    if not sched_ok:
        print(json.dumps({"status": "error", "step_failed": "SCHEDULE", "details": sched_err}), flush=True)
        return
    print(json.dumps({
        "status": "success",
        "public_url": f"https://{hostname}",
        "message": f"Tunnel Zero Trust établi. Un code PIN sera envoyé à {email}.",
        "ttl_seconds": int(ttl),
    }), flush=True)


if __name__ == "__main__":
    main()
