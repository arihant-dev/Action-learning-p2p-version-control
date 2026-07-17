#!/usr/bin/env python3
"""Container entrypoint for a single P2P peer.

Starts the Go coordinator, waits for its IPC listener (TCP mode), and sends
an add_repository message if REPO_ID/REPO_PATH are configured. The coordinator
will then spawn the C++ watcher daemon for that repository.
"""
import json
import os
import signal
import socket
import struct
import subprocess
import sys
import threading
import time


def log(msg):
    print(f"[peer-entrypoint] {msg}", flush=True)


def send_ipc_message(host, port, msg_type, payload):
    message = {
        "version": "1.0",
        "type": msg_type,
        "source": "peer-entrypoint",
        "timestamp": int(time.time() * 1000),
        "payload": payload,
    }
    data = json.dumps(message).encode("utf-8")
    length_prefix = struct.pack(">I", len(data))

    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(5.0)
    s.connect((host, port))
    s.sendall(length_prefix + data)
    s.close()


def wait_for_ipc(host, port, timeout=10.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            s.settimeout(1.0)
            s.connect((host, port))
            s.close()
            return True
        except (OSError, socket.timeout):
            time.sleep(0.2)
    return False


def main():
    peer_id = os.environ.get("PEER_ID", "peer")
    ipc_port = os.environ.get("IPC_TCP_PORT")
    if not ipc_port:
        log("ERROR: IPC_TCP_PORT is required")
        sys.exit(1)
    ipc_port = int(ipc_port)

    repo_id = os.environ.get("REPO_ID")
    repo_path = os.environ.get("REPO_PATH")
    if repo_id and repo_path:
        os.makedirs(repo_path, exist_ok=True)

    log(f"Starting coordinator for {peer_id} (IPC TCP port {ipc_port})")
    coord_env = os.environ.copy()
    coord_proc = subprocess.Popen(
        ["/usr/local/bin/go_coordinator"],
        env=coord_env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )

    def forward_logs():
        for line in coord_proc.stdout:
            sys.stdout.buffer.write(line)
            sys.stdout.flush()

    log_thread = threading.Thread(target=forward_logs, daemon=True)
    log_thread.start()

    def shutdown(signum, frame):
        log("Shutting down coordinator...")
        coord_proc.terminate()
        try:
            coord_proc.wait(timeout=5.0)
        except subprocess.TimeoutExpired:
            coord_proc.kill()
        sys.exit(0)

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)

    if not wait_for_ipc("127.0.0.1", ipc_port):
        log("ERROR: Coordinator IPC did not become ready")
        coord_proc.kill()
        sys.exit(1)

    if repo_id and repo_path:
        log(f"Adding repository {repo_id} at {repo_path}")
        send_ipc_message("127.0.0.1", ipc_port, "add_repository", {
            "repo_id": repo_id,
            "path": repo_path,
        })

    log("Coordinator ready, waiting for termination signal")
    coord_proc.wait()


if __name__ == "__main__":
    main()
