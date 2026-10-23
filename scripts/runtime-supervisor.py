#!/usr/bin/env python3
"""Supervise the existing single-container source deployment (not Compose).

If either service exits, stop its sibling and fail the container so Docker's
unless-stopped policy can restart both. No installation/build happens at boot.
"""
import os
import json
import signal
import subprocess
import time
from pathlib import Path


def supervise(commands, log_dir, grace=90):
    children = []
    logs = []
    stopping = False

    def request_stop(_signal, _frame):
        nonlocal stopping
        stopping = True

    previous = {sig: signal.signal(sig, request_stop) for sig in (signal.SIGTERM, signal.SIGINT)}
    try:
        Path(log_dir).mkdir(parents=True, exist_ok=True)
        for name, command, cwd in commands:
            log = open(Path(log_dir) / (name + ".log"), "ab", buffering=0)
            logs.append(log)
            child = subprocess.Popen(command, cwd=cwd, stdin=subprocess.DEVNULL,
                                     stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            children.append(child)
            print(f"Started {name}: pid={child.pid}", flush=True)
        while not stopping:
            if any(child.poll() is not None for child in children):
                print("A runtime service exited; stopping siblings for container recovery", flush=True)
                return 1
            time.sleep(0.2)
        return 0
    finally:
        # Groups are created and owned by this supervisor; never search/kill by name.
        for child in children:
            try:
                os.killpg(child.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
        deadline = time.monotonic() + grace
        for child in children:
            try:
                child.wait(timeout=max(0.1, deadline - time.monotonic()))
            except subprocess.TimeoutExpired:
                pass
        for child in children:
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            child.wait()
        for log in logs:
            log.close()
        for sig, handler in previous.items():
            signal.signal(sig, handler)


if __name__ == "__main__":
    env_file = os.environ.get("CANVAS_RUNTIME_ENV_FILE")
    if env_file:
        # The runtime file is outside Git and must be readable only by operators.
        values = json.loads(Path(env_file).read_text())
        if not isinstance(values, dict) or any(not isinstance(v, str) for v in values.values()):
            raise ValueError("Invalid runtime environment file")
        os.environ.update(values)
    app = Path(os.environ.get("CANVAS_APP_DIR", "/app"))
    runtime = Path(os.environ.get("CANVAS_RUNTIME_DIR", "/runtime"))
    raise SystemExit(supervise([
        ("backend", [os.environ.get("CANVAS_BACKEND_BINARY", str(runtime / "tools/canvas-backend"))], app / "backend"),
        ("frontend", [str(runtime / "tools/bun/bun"), "run", "start"], app / "web"),
    ], runtime / "logs"))
