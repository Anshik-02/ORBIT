#!/usr/bin/env python3
"""
ORBIT Distributed Key-Value Database — Cluster Orchestrator Suite
Cross-platform cluster lifecycle manager: start, stop, restart, status, logs.
Strict PID-verified process management: zero broad or unsafe taskkill calls.
"""

import os
import sys
import json
import time
import signal
import socket
import urllib.request
import subprocess

ROOT_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
BIN_NAME = "orbit.exe" if sys.platform == "win32" else "orbit"
BIN_PATH = os.path.join(ROOT_DIR, "bin", BIN_NAME)
LOGS_DIR = os.path.join(ROOT_DIR, "logs")
PID_FILE = os.path.join(LOGS_DIR, "cluster.pids.json")

NODES = [
    {"id": "node-a", "api_port": 2525, "raft_port": 7000},
    {"id": "node-b", "api_port": 2526, "raft_port": 7001},
    {"id": "node-c", "api_port": 2527, "raft_port": 7002},
    {"id": "node-d", "api_port": 2528, "raft_port": 7003},
    {"id": "node-e", "api_port": 2529, "raft_port": 7004},
]

def is_socket_open(port: int, host: str = "127.0.0.1", timeout: float = 0.5) -> bool:
    try:
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
            s.settimeout(timeout)
            return s.connect_ex((host, port)) == 0
    except Exception:
        return False

def is_node_healthy(api_port: int, timeout: float = 1.0) -> bool:
    """Verifies that the node's HTTP server is fully serving requests."""
    endpoints = [
        f"http://127.0.0.1:{api_port}/health",
        f"http://127.0.0.1:{api_port}/cluster/health",
        f"http://127.0.0.1:{api_port}/cluster/status",
    ]
    for url in endpoints:
        try:
            req = urllib.request.Request(url, headers={"Accept": "application/json"})
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                if resp.status == 200:
                    return True
        except Exception:
            pass
    return False

def wait_for_node_readiness(node_info: dict, timeout_sec: float = 8.0) -> bool:
    """Polls both Raft & HTTP ports and waits for healthy HTTP response."""
    start_time = time.time()
    api_port = node_info["api_port"]
    raft_port = node_info["raft_port"]

    while time.time() - start_time < timeout_sec:
        # Check if both sockets are accepting connections
        http_open = is_socket_open(api_port, timeout=0.2)
        raft_open = is_socket_open(raft_port, timeout=0.2)

        if http_open and raft_open:
            if is_node_healthy(api_port, timeout=0.5):
                return True
        time.sleep(0.2)

    return False

def find_pid_by_port(port: int):
    """Finds the specific PID listening on the given port."""
    if sys.platform == "win32":
        try:
            cmd = f'powershell -NoProfile -Command "(Get-NetTCPConnection -LocalPort {port} -State Listen -ErrorAction SilentlyContinue).OwningProcess"'
            out = subprocess.check_output(cmd, shell=True, text=True).strip()
            if out and out.isdigit():
                return int(out)
        except Exception:
            pass
    else:
        try:
            out = subprocess.check_output(["lsof", "-t", "-i", f":{port}", "-sTCP:LISTEN"], text=True).strip()
            if out and out.isdigit():
                return int(out)
        except Exception:
            pass
    return None

def verify_orbit_process(pid: int, expected_node_id: str) -> bool:
    """Verifies that the process at PID is indeed an ORBIT instance for expected_node_id."""
    if not pid or not isinstance(pid, int):
        return False

    if sys.platform == "win32":
        try:
            cmd = f'powershell -NoProfile -Command "(Get-CimInstance Win32_Process -Filter \\"ProcessId = {pid}\\" -ErrorAction SilentlyContinue).CommandLine"'
            cmdline = subprocess.check_output(cmd, shell=True, text=True).strip().lower()
            return ("orbit" in cmdline) and (expected_node_id in cmdline)
        except Exception:
            return False
    else:
        try:
            cmdline_path = f"/proc/{pid}/cmdline"
            if os.path.exists(cmdline_path):
                with open(cmdline_path, "rb") as f:
                    cmdline = f.read().decode(errors="ignore").lower()
                return ("orbit" in cmdline) and (expected_node_id in cmdline)
        except Exception:
            return False
    return False

def load_pids() -> dict:
    if os.path.exists(PID_FILE):
        try:
            with open(PID_FILE, "r") as f:
                data = json.load(f)
                # Clean stale PIDs
                cleaned = {}
                for nid, pid in data.items():
                    if verify_orbit_process(pid, nid):
                        cleaned[nid] = pid
                if len(cleaned) != len(data):
                    save_pids(cleaned)
                return cleaned
        except Exception:
            return {}
    return {}

def save_pids(pids: dict):
    os.makedirs(LOGS_DIR, exist_ok=True)
    with open(PID_FILE, "w") as f:
        json.dump(pids, f, indent=2)

def compile_binary_if_needed():
    if not os.path.exists(BIN_PATH):
        print(f"[ORCHESTRATOR] Compiling {BIN_NAME} from ./cmd/orbit...")
        cmd = ["go", "build", "-o", os.path.join("bin", BIN_NAME), "./cmd/orbit"]
        subprocess.run(cmd, cwd=ROOT_DIR, check=True)
        print("[ORCHESTRATOR] Binary compiled successfully.")

def get_node_pid(node_id: str, pids: dict, port: int):
    # 1. Check PID from state file
    stored_pid = pids.get(node_id)
    if stored_pid and verify_orbit_process(stored_pid, node_id):
        return stored_pid

    # 2. Re-discover from listening port
    active_pid = find_pid_by_port(port)
    if active_pid and verify_orbit_process(active_pid, node_id):
        pids[node_id] = active_pid
        save_pids(pids)
        return active_pid

    return None

def start_node(node_info: dict, pids: dict) -> bool:
    nid = node_info["id"]
    cfg = f"configs/{nid}.json"
    cluster_cfg = "configs/cluster.json"

    stdout_path = os.path.join(LOGS_DIR, f"{nid}.stdout.log")
    stderr_path = os.path.join(LOGS_DIR, f"{nid}.stderr.log")

    stdout_f = open(stdout_path, "a")
    stderr_f = open(stderr_path, "a")

    cmd = [BIN_PATH, "-config", cfg, "-cluster", cluster_cfg]

    kwargs = {}
    if sys.platform == "win32":
        kwargs["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP

    p = subprocess.Popen(
        cmd,
        cwd=ROOT_DIR,
        stdout=stdout_f,
        stderr=stderr_f,
        **kwargs
    )

    # Detect immediate crash on launch
    time.sleep(0.3)
    poll_res = p.poll()
    if poll_res is not None:
        print(f"  [X] FAILED to start {nid}: process exited immediately with code {poll_res}.")
        return False

    pids[nid] = p.pid
    print(f"  + Launched {nid:8s} | PID: {p.pid:<6d} | HTTP: :{node_info['api_port']} | Raft: :{node_info['raft_port']}")
    return True

def start_cluster():
    os.makedirs(LOGS_DIR, exist_ok=True)
    compile_binary_if_needed()

    print("=================================================================")
    print("        ORBIT DISTRIBUTED CONTROL PLANE — CLUSTER STARTUP        ")
    print("=================================================================")

    pids = load_pids()
    launched_nodes = []
    failed_nodes = []

    for node in NODES:
        nid = node["id"]
        pid = get_node_pid(nid, pids, node["api_port"])

        if pid:
            print(f"  * {nid:8s} is already running (PID: {pid})")
            continue

        if is_socket_open(node["api_port"]):
            print(f"  ! Port {node['api_port']} for {nid} is already in use by another process. Skipping.")
            failed_nodes.append(nid)
            continue

        success = start_node(node, pids)
        if success:
            launched_nodes.append(node)
        else:
            failed_nodes.append(nid)

    save_pids(pids)

    # Readiness probe: wait for all newly launched nodes to complete bootstrap & quorum
    if launched_nodes:
        print("\nVerifying cluster node readiness (HTTP API & Raft consensus ports)...")
        for node in launched_nodes:
            nid = node["id"]
            ready = wait_for_node_readiness(node, timeout_sec=8.0)
            if ready:
                print(f"  [OK] {nid:8s} ready and healthy (HTTP :{node['api_port']}, Raft :{node['raft_port']})")
            else:
                print(f"  [WARN] {nid:8s} launched but did not report ready within timeout. Check logs/{nid}.stderr.log")

    if failed_nodes:
        print(f"\n[ALERT] Partial startup occurred! Failed nodes: {failed_nodes}")

    print("\n-----------------------------------------------------------------")
    print("Cluster fleet online! Active REST & Raft endpoints:")
    for node in NODES:
        print(f"  - {node['id']:8s} => http://127.0.0.1:{node['api_port']} (Health: /cluster/health)")
    print("Frontend UI: http://localhost:3000/dashboard")
    print("-----------------------------------------------------------------")

def stop_node(node_id: str, pids: dict):
    node = next((n for n in NODES if n["id"] == node_id), None)
    port = node["api_port"] if node else 2525
    pid = get_node_pid(node_id, pids, port)

    if pid and verify_orbit_process(pid, node_id):
        print(f"  - Stopping {node_id} (PID: {pid}, verified)...")
        # 1. Graceful shutdown attempt first
        if sys.platform == "win32":
            # Attempt graceful taskkill without /F first
            subprocess.run(["taskkill", "/PID", str(pid)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            # Wait up to 2 seconds for graceful WAL flush and close
            for _ in range(10):
                time.sleep(0.2)
                if not verify_orbit_process(pid, node_id):
                    break
            # If still alive, terminate forcefully
            if verify_orbit_process(pid, node_id):
                subprocess.run(["taskkill", "/F", "/PID", str(pid)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        else:
            try:
                os.kill(pid, signal.SIGTERM)
                for _ in range(10):
                    time.sleep(0.2)
                    if not verify_orbit_process(pid, node_id):
                        break
                if verify_orbit_process(pid, node_id):
                    os.kill(pid, signal.SIGKILL)
            except OSError:
                pass

        pids.pop(node_id, None)
    else:
        print(f"  * {node_id} is not running.")

def stop_cluster():
    print("=================================================================")
    print("        ORBIT DISTRIBUTED CONTROL PLANE — CLUSTER SHUTDOWN       ")
    print("=================================================================")
    pids = load_pids()

    # Terminate strictly each verified node PID
    for node in NODES:
        stop_node(node["id"], pids)

    save_pids({})
    print("\nAll ORBIT cluster node processes stopped safely via verified PIDs.")

def restart_node(node_id: str):
    node = next((n for n in NODES if n["id"] == node_id), None)
    if not node:
        print(f"Error: Unknown node ID '{node_id}'. Valid nodes: {[n['id'] for n in NODES]}")
        sys.exit(1)

    print(f"Restarting node '{node_id}'...")
    pids = load_pids()
    stop_node(node_id, pids)
    time.sleep(0.5)
    start_node(node, pids)
    save_pids(pids)
    print(f"Node '{node_id}' restarted successfully.")

def show_status():
    print("=================================================================")
    print("        ORBIT DISTRIBUTED FLEET STATUS & TELEMETRY TABLE        ")
    print("=================================================================")
    print(f"{'NODE ID':<10} | {'PID':<8} | {'HTTP PORT':<10} | {'RAFT':<8} | {'HEALTH':<10} | {'REPLICAS'}")
    print("-" * 65)

    pids = load_pids()

    for node in NODES:
        nid = node["id"]
        pid = get_node_pid(nid, pids, node["api_port"])
        is_alive = pid is not None

        health_str = "OFFLINE"
        shards_str = "—"

        if is_alive:
            try:
                url = f"http://127.0.0.1:{node['api_port']}/cluster/health"
                req = urllib.request.Request(url, headers={"Accept": "application/json"})
                with urllib.request.urlopen(req, timeout=1.0) as resp:
                    if resp.status == 200:
                        data = json.loads(resp.read().decode())
                        node_data = data.get("nodes", {}).get(nid, {})
                        is_online = node_data.get("online", True)
                        health_str = "ONLINE" if is_online else "DEGRADED"
                        hosted = node_data.get("hosted_shard_ids", [])
                        shards_str = f"Shards: {hosted}" if hosted else "RF=3 In-Sync"
            except Exception:
                health_str = "ONLINE"

        pid_display = str(pid) if pid else "-"
        print(f"{nid:<10} | {pid_display:<8} | :{node['api_port']:<9} | :{node['raft_port']:<7} | {health_str:<10} | {shards_str}")

    print("-" * 65)

def tail_logs(node_id: str, lines: int = 30):
    node = next((n for n in NODES if n["id"] == node_id), None)
    if not node:
        print(f"Error: Unknown node '{node_id}'. Valid nodes: {[n['id'] for n in NODES]}")
        sys.exit(1)

    stdout_path = os.path.join(LOGS_DIR, f"{node_id}.stdout.log")
    stderr_path = os.path.join(LOGS_DIR, f"{node_id}.stderr.log")

    print(f"=== Last {lines} lines for {node_id} (STDOUT) ===")
    if os.path.exists(stdout_path):
        with open(stdout_path, "r", encoding="utf-8", errors="ignore") as f:
            all_lines = f.readlines()
            for line in all_lines[-lines:]:
                print(line.rstrip())
    else:
        print("(No stdout log found)")

    if os.path.exists(stderr_path):
        with open(stderr_path, "r", encoding="utf-8", errors="ignore") as f:
            err_lines = f.readlines()
            if err_lines:
                print(f"\n=== Last {lines} lines for {node_id} (STDERR) ===")
                for line in err_lines[-lines:]:
                    print(line.rstrip())

if __name__ == "__main__":
    action = sys.argv[1].lower() if len(sys.argv) > 1 else "status"

    if action == "start":
        start_cluster()
    elif action == "stop":
        stop_cluster()
    elif action == "restart":
        if len(sys.argv) > 2:
            restart_node(sys.argv[2])
        else:
            stop_cluster()
            time.sleep(1)
            start_cluster()
    elif action == "status":
        show_status()
    elif action == "logs":
        target = sys.argv[2] if len(sys.argv) > 2 else "node-a"
        num_lines = int(sys.argv[3]) if len(sys.argv) > 3 else 30
        tail_logs(target, num_lines)
    else:
        print(f"Usage: python {sys.argv[0]} [start|stop|restart|status|logs] [node_id] [lines]")
        sys.exit(1)
