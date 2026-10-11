#!/usr/bin/env python3
"""Local demo orchestration. Requires only Python 3 and Docker Compose."""
import argparse
import base64
import json
import os
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

HERE = Path(__file__).resolve().parent
ENV_FILE = HERE / ".env"
GIT_URL = "http://127.0.0.1:13000"
API_URL = "http://127.0.0.1:18880"
USER = "demo"
PASSWORD = "docker-cd-demo-2026"
REPO = "/repos/demo/homelab"
WEBHOOK_SECRET = "docker-cd-demo-webhook"


def environment():
    if not ENV_FILE.exists():
        return {}
    return dict(line.split("=", 1) for line in ENV_FILE.read_text().splitlines()
                if line and not line.startswith("#") and "=" in line)


def save_environment(values):
    content = "# Generated local-demo configuration; no production credentials.\n"
    content += "".join(f"{key}={value}\n" for key, value in values.items())
    fd = os.open(ENV_FILE, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w") as target:
        target.write(content)


def compose(*args):
    if not ENV_FILE.exists():
        save_environment({"DEMO_DRIFT_POLICY": "revert"})
    command = ["docker", "compose", "--project-name", "docker-cd-demo",
               "--env-file", str(ENV_FILE), "--file", str(HERE / "compose.yaml"), *args]
    subprocess.run(command, check=True)


def request(url, method="GET", body=None, auth=False):
    headers = {"Content-Type": "application/json"}
    if auth:
        value = base64.b64encode(f"{USER}:{PASSWORD}".encode()).decode()
        headers["Authorization"] = f"Basic {value}"
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=30) as response:
        data = response.read()
        return json.loads(data) if data else None


def git(path, method="GET", body=None):
    return request(GIT_URL + "/api/v1" + path, method, body, auth=True)


def exists(path, auth=True):
    try:
        return request(GIT_URL + "/api/v1" + path, auth=auth)
    except urllib.error.HTTPError as error:
        if error.code == 404:
            return None
        raise


def put_file(path, content, message):
    endpoint = REPO + "/contents/" + urllib.parse.quote(path, safe="/")
    current = exists(endpoint)
    body = {"branch": "main", "message": message,
            "content": base64.b64encode(content.encode()).decode()}
    if current:
        body["sha"] = current["sha"]
    result = git(endpoint, "PUT" if current else "POST", body)
    print(f"Committed {path}: {result['commit']['sha'][:12]}", flush=True)


def bootstrap():
    if not exists("/users/demo", auth=False):
        compose("exec", "-T", "--user", "git", "git", "gitea", "admin", "user", "create",
                "--config", "/data/gitea/conf/app.ini", "--username", USER,
                "--password", PASSWORD, "--email", "demo@example.test", "--admin",
                "--must-change-password=false")
    values = environment()
    if not values.get("DEMO_GIT_TOKEN"):
        token = git("/users/demo/tokens", "POST", {
            "name": "docker-cd-demo-" + str(time.time_ns()), "scopes": ["read:repository"]})
        values["DEMO_GIT_TOKEN"] = token["sha1"]
        save_environment(values)
    if not exists(REPO):
        git("/user/repos", "POST", {"name": "homelab", "private": False,
            "auto_init": True, "default_branch": "main", "description": "Docker-CD disposable GitOps demo"})
        for file in sorted((HERE / "seed").rglob("compose.yaml")):
            path = file.relative_to(HERE / "seed").as_posix()
            put_file(path, file.read_text(), "Seed " + path)
        put_file("README.md", (HERE / "repo-readme.md").read_text(), "Add demo instructions")
    else:
        print("Existing demo repository preserved (including manual edits).", flush=True)
    hooks = git(REPO + "/hooks")
    target = "http://backend:8080/api/webhook"
    if not any(hook.get("config", {}).get("url") == target for hook in hooks):
        git(REPO + "/hooks", "POST", {"type": "gitea", "active": True,
            "events": ["push"], "branch_filter": "main",
            "config": {"url": target, "content_type": "json", "secret": WEBHOOK_SECRET}})


def wait_for_stacks():
    deadline = time.monotonic() + 240
    while time.monotonic() < deadline:
        try:
            refresh = request(API_URL + "/api/refresh-status")
            stacks = request(API_URL + "/api/stacks")
            if refresh["refreshStatus"] == "completed" and stacks and all(
                    stack["status"] not in ("missing", "syncing") for stack in stacks):
                for stack in stacks:
                    print(f"  {stack['path']}: {stack['status']}", flush=True)
                return
        except (urllib.error.URLError, TimeoutError):
            pass
        time.sleep(2)
    raise RuntimeError("Demo did not finish initial deployment; inspect: python3 demo/manage.py logs")


def links():
    print("\nDocker-CD:  http://localhost:18881")
    print("Git repo:   http://localhost:13000/demo/homelab")
    print(f"Git login:  {USER} / {PASSWORD}")
    print("Welcome:    http://localhost:18088")
    print("Backend:    http://localhost:18880/api/instance")


def start():
    compose("up", "-d", "--build", "--wait", "--wait-timeout", "180", "git", "git-https", "dind")
    bootstrap()
    compose("up", "-d", "--build", "--wait", "--wait-timeout", "240", "backend", "frontend")
    print("Waiting for initial stack deployment…", flush=True)
    wait_for_stacks()
    links()


def scenario(name):
    if name in ("change", "approval"):
        if name == "approval":
            values = environment()
            values["DEMO_DRIFT_POLICY"] = "flag"
            save_environment(values)
            compose("up", "-d", "--wait", "backend")
            wait_for_stacks()
        file = HERE / "seed/apps/welcome/compose.yaml"
        content = file.read_text().replace("DEMO_VERSION: v1", f"DEMO_VERSION: v{int(time.time())}")
        put_file("apps/welcome/compose.yaml", content, "Demo: change welcome version")
    elif name in ("break", "recover"):
        content = (HERE / "seed/apps/broken/compose.yaml").read_text()
        if name == "recover":
            content = content.replace("    deliberately_invalid_demo_property: true\n", "")
        put_file("apps/broken/compose.yaml", content, "Demo: " + name + " broken stack")
    elif name == "healthy":
        content = (HERE / "seed/apps/unhealthy/compose.yaml").read_text().replace('[CMD, "false"]', '[CMD, "true"]')
        put_file("apps/unhealthy/compose.yaml", content, "Demo: fix runtime health check")
    elif name == "remove":
        path = REPO + "/contents/infra/heartbeat/compose.yaml"
        file = exists(path)
        if file:
            git(path, "DELETE", {"sha": file["sha"], "branch": "main", "message": "Demo: remove heartbeat"})
            print("Committed heartbeat removal.")
    elif name == "restore":
        put_file("infra/heartbeat/compose.yaml", (HERE / "seed/infra/heartbeat/compose.yaml").read_text(), "Demo: restore heartbeat")
    print("Watch Operations in Docker-CD; Gitea will deliver the signed push webhook.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["start", "stop", "down", "reset", "status", "logs", "scenario", "policy"])
    parser.add_argument("value", nargs="?")
    args = parser.parse_args()
    if args.command == "start":
        start()
    elif args.command == "stop":
        compose("stop")
    elif args.command == "down":
        compose("down")
    elif args.command == "reset":
        compose("down", "--volumes")
        if ENV_FILE.exists():
            ENV_FILE.unlink()
        start()
    elif args.command == "status":
        compose("ps")
        links()
    elif args.command == "logs":
        compose("logs", "--tail=150", args.value or "backend")
    elif args.command == "policy":
        if args.value not in ("flag", "revert"):
            parser.error("policy requires flag or revert")
        values = environment()
        values["DEMO_DRIFT_POLICY"] = args.value
        save_environment(values)
        compose("up", "-d", "--wait", "backend")
    else:
        if args.value not in ("change", "approval", "break", "recover", "healthy", "remove", "restore"):
            parser.error("scenario requires change, approval, break, recover, healthy, remove, or restore")
        scenario(args.value)


if __name__ == "__main__":
    try:
        main()
    except urllib.error.HTTPError as error:
        raise SystemExit(f"Demo Git API returned HTTP {error.code}: {error.read().decode()}")
    except subprocess.CalledProcessError as error:
        raise SystemExit(f"Demo Compose command failed ({error.returncode}); inspect python3 demo/manage.py logs")
