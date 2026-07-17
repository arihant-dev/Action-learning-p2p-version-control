#!/usr/bin/env python3
"""Containerized multi-peer network test harness.

Runs isolated P2P sync scenarios using docker-compose.test.yml. The
orchestrator executes on the host and drives the peers via docker compose exec
and docker network commands.
"""
import json
import os
import subprocess
import sys
import time

COMPOSE_FILE = "docker-compose.test.yml"

RESULTS = {"passed": 0, "failed": 0, "skipped": 0}


def log(msg):
    print(f"[DockerHarness] {msg}", flush=True)


def run(cmd, check=True, **kwargs):
    log(f"$ {' '.join(cmd)}")
    return subprocess.run(cmd, check=check, **kwargs)


def compose_up():
    run(["docker", "compose", "-f", COMPOSE_FILE, "up", "--build", "-d"])


def compose_down():
    run(["docker", "compose", "-f", COMPOSE_FILE, "down", "-v"], check=False)


def compose_exec(service, cmd, check=True, capture=False, text=True):
    full = ["docker", "compose", "-f", COMPOSE_FILE, "exec", "-T", service] + cmd
    return run(full, check=check, capture_output=capture, text=text)


def health(service, health_port):
    code = (
        f'import urllib.request, json; '
        f'print(urllib.request.urlopen("http://127.0.0.1:{health_port}/health", timeout=2).read().decode())'
    )
    try:
        result = compose_exec(service, ["python3", "-c", code], capture=True, check=True)
        return json.loads(result.stdout)
    except Exception as e:
        log(f"health check for {service} failed: {e}")
        return None


def wait_for_health(service, health_port, timeout=60.0):
    log(f"Waiting for {service} health on port {health_port}...")
    deadline = time.time() + timeout
    while time.time() < deadline:
        h = health(service, health_port)
        if h and h.get("status") == "ok":
            log(f"{service} is healthy")
            return True
        time.sleep(0.5)
    log(f"{service} did not become healthy within {timeout}s")
    return False


def wait_for_connections(service, health_port, min_connections=1, timeout=30.0):
    log(f"Waiting for {service} to have {min_connections} connection(s)...")
    deadline = time.time() + timeout
    while time.time() < deadline:
        h = health(service, health_port)
        if h and h.get("connections", 0) >= min_connections:
            log(f"{service} has {h.get('connections')} connection(s)")
            return True
        time.sleep(0.5)
    log(f"{service} did not reach {min_connections} connection(s) in time")
    return False


def write_file(service, path, content):
    code = (
        f'import os; '
        f'os.makedirs(os.path.dirname({repr(path)}), exist_ok=True); '
        f'open({repr(path)}, "w").write({repr(content)})'
    )
    compose_exec(service, ["python3", "-c", code], check=True)


def read_file(service, path):
    result = compose_exec(service, ["cat", path], capture=True, check=False)
    if result.returncode == 0:
        return result.stdout
    return None


def file_exists(service, path, expected_content=None, timeout=30.0):
    log(f"Waiting for {path} on {service}...")
    deadline = time.time() + timeout
    while time.time() < deadline:
        content = read_file(service, path)
        if content is not None:
            if expected_content is None or content == expected_content:
                return True
        time.sleep(0.5)
    return False


def network_disconnect(service):
    log(f"Disconnecting {service} from p2p-test network")
    run(["docker", "network", "disconnect", "p2p-test", service])


def network_connect(service):
    log(f"Reconnecting {service} to p2p-test network")
    run(["docker", "network", "connect", "p2p-test", service])


def test_basic_two_peer_sync():
    log("\n=== TEST: Basic Two-Peer Sync (Docker) ===")
    if not wait_for_health("peer-a", 10981):
        return False
    if not wait_for_health("peer-b", 10982):
        return False
    if not wait_for_connections("peer-a", 10981, min_connections=1):
        return False
    if not wait_for_connections("peer-b", 10982, min_connections=1):
        return False

    content = "Hello from containerized peer-a"
    write_file("peer-a", "/data/repo/synced.txt", content)

    if file_exists("peer-b", "/data/repo/synced.txt", content, timeout=30.0):
        log("TEST PASSED: Basic Two-Peer Sync")
        return True
    log("TEST FAILED: file did not sync to peer-b")
    return False


def test_three_peer_sync():
    log("\n=== TEST: Three-Peer Sync (Docker) ===")
    for peer, port in [("peer-1", 10990), ("peer-2", 10991), ("peer-3", 10992)]:
        if not wait_for_health(peer, port):
            return False
    for peer, port in [("peer-1", 10990), ("peer-2", 10991), ("peer-3", 10992)]:
        if not wait_for_connections(peer, port, min_connections=2):
            return False

    content = "Three-peer container sync"
    write_file("peer-1", "/data/repo/shared.txt", content)

    ok2 = file_exists("peer-2", "/data/repo/shared.txt", content, timeout=30.0)
    ok3 = file_exists("peer-3", "/data/repo/shared.txt", content, timeout=30.0)
    if ok2 and ok3:
        log("TEST PASSED: Three-Peer Sync")
        return True
    log(f"TEST FAILED: peer-2={ok2}, peer-3={ok3}")
    return False


def test_network_partition():
    log("\n=== TEST: Network Partition (Docker) ===")
    if not wait_for_health("peer-partition-a", 10920):
        return False
    if not wait_for_health("peer-partition-b", 10921):
        return False
    if not wait_for_connections("peer-partition-a", 10920, min_connections=1):
        return False
    if not wait_for_connections("peer-partition-b", 10921, min_connections=1):
        return False

    # Sync a baseline file first
    baseline = "before partition"
    write_file("peer-partition-a", "/data/repo/baseline.txt", baseline)
    if not file_exists("peer-partition-b", "/data/repo/baseline.txt", baseline, timeout=20.0):
        log("TEST FAILED: baseline file did not sync before partition")
        return False

    # Partition
    network_disconnect("peer-partition-b")
    time.sleep(2.0)

    # Create file while partitioned
    partitioned = "created during partition"
    write_file("peer-partition-a", "/data/repo/partitioned.txt", partitioned)
    time.sleep(2.0)

    # Heal
    network_connect("peer-partition-b")
    if not wait_for_connections("peer-partition-a", 10920, min_connections=1, timeout=30.0):
        return False

    if file_exists("peer-partition-b", "/data/repo/partitioned.txt", partitioned, timeout=40.0):
        log("TEST PASSED: Network Partition")
        return True
    log("TEST FAILED: partitioned file did not sync after heal")
    return False


def run_test(name, fn):
    log(f"\n--- Starting test: {name} ---")
    try:
        result = fn()
        if result:
            RESULTS["passed"] += 1
            log(f"--- {name}: PASSED ---")
        else:
            RESULTS["failed"] += 1
            log(f"--- {name}: FAILED ---")
    except Exception as e:
        RESULTS["failed"] += 1
        log(f"--- {name}: ERROR - {e} ---")


def main():
    log("Cleaning up any previous test stack...")
    compose_down()

    log("Building and starting peer stack...")
    compose_up()

    try:
        run_test("basic_two_peer_sync", test_basic_two_peer_sync)
        run_test("three_peer_sync", test_three_peer_sync)
        run_test("network_partition", test_network_partition)
    finally:
        log("\nCollecting logs...")
        run(["docker", "compose", "-f", COMPOSE_FILE, "logs", "--tail", "100"], check=False)
        log("Tearing down peer stack...")
        compose_down()

    total = RESULTS["passed"] + RESULTS["failed"]
    log(f"\n{'='*60}")
    log(f"RESULTS: {RESULTS['passed']}/{total} passed, {RESULTS['failed']} failed, {RESULTS['skipped']} skipped")
    log(f"{'='*60}")

    if RESULTS["failed"] > 0:
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
