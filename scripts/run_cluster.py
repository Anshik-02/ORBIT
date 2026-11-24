import os
import sys
import subprocess
import time

ROOT_DIR = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
BIN_PATH = os.path.join(ROOT_DIR, "bin", "orbit.exe")
LOGS_DIR = os.path.join(ROOT_DIR, "logs")

NODES = ["node-a", "node-b", "node-c", "node-d", "node-e"]

def start_cluster():
    os.makedirs(LOGS_DIR, exist_ok=True)
    if not os.path.exists(BIN_PATH):
        print("Compiling bin/orbit.exe...")
        subprocess.run(["go", "build", "-o", "bin/orbit.exe", "./cmd/orbit"], cwd=ROOT_DIR, check=True)

    print("==========================================================")
    print(" ORBIT Distributed Key-Value Database — Cluster Launcher  ")
    print("==========================================================")
    
    procs = []
    for node in NODES:
        cfg = f"configs/{node}.json"
        cluster_cfg = "configs/cluster.json"
        stdout_f = open(os.path.join(LOGS_DIR, f"{node}.stdout.log"), "w")
        stderr_f = open(os.path.join(LOGS_DIR, f"{node}.stderr.log"), "w")
        
        cmd = [BIN_PATH, "-config", cfg, "-cluster", cluster_cfg]
        p = subprocess.Popen(
            cmd,
            cwd=ROOT_DIR,
            stdout=stdout_f,
            stderr=stderr_f,
            creationflags=subprocess.CREATE_NEW_PROCESS_GROUP
        )
        procs.append((node, p.pid))
        print(f"Started {node} (PID: {p.pid})")

    print("\nAll 5 nodes launched in background!")
    print("Endpoints:")
    for idx, node in enumerate(NODES):
        port = 2525 + idx
        print(f"  - {node}: http://127.0.0.1:{port}")
    print("\nFrontend Dashboard: http://localhost:3000/dashboard")

def stop_cluster():
    print("Stopping all ORBIT processes...")
    if sys.platform == "win32":
        subprocess.run(["taskkill", "/F", "/IM", "orbit.exe", "/T"], stderr=subprocess.DEVNULL, stdout=subprocess.DEVNULL)
    else:
        subprocess.run(["pkill", "-f", "orbit"], stderr=subprocess.DEVNULL, stdout=subprocess.DEVNULL)
    print("All nodes stopped.")

if __name__ == "__main__":
    action = sys.argv[1] if len(sys.argv) > 1 else "start"
    if action == "stop":
        stop_cluster()
    else:
        start_cluster()
