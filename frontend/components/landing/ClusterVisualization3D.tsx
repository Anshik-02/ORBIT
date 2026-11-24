"use client";

import React, { useEffect, useRef, useState } from "react";
import * as THREE from "three";
import { ShieldCheck, Activity, Layers, Radio, Sparkles, Server, Crown, AlertTriangle } from "lucide-react";
import { fetchClusterHealth } from "@/lib/api/client";
import { ClusterHealthResponse } from "@/lib/api/types";

interface NodeData {
  id: string;
  name: string;
  role: "Leader" | "Follower";
  pos: [number, number, number];
  color: string;
  shards: number[];
  apiPort: number;
  raftPort: number;
  term: number;
  status: "Healthy" | "Replicating" | "Elected" | "Offline";
}

const NODES: NodeData[] = [
  {
    id: "node-a",
    name: "Node A",
    role: "Follower",
    pos: [0, 2.1, 0.2],
    color: "#0891D5", // Primary Cyan
    shards: [0, 1],
    apiPort: 2525,
    raftPort: 7000,
    term: 0,
    status: "Healthy",
  },
  {
    id: "node-b",
    name: "Node B",
    role: "Follower",
    pos: [-2.5, 0.7, 0.4],
    color: "#7965E8", // Secondary Purple
    shards: [0, 2],
    apiPort: 2526,
    raftPort: 7001,
    term: 0,
    status: "Healthy",
  },
  {
    id: "node-c",
    name: "Node C",
    role: "Follower",
    pos: [2.5, 0.7, -0.4],
    color: "#0891D5",
    shards: [0, 2],
    apiPort: 2527,
    raftPort: 7002,
    term: 0,
    status: "Healthy",
  },
  {
    id: "node-d",
    name: "Node D",
    role: "Follower",
    pos: [-1.7, -1.9, -0.3],
    color: "#10B981", // Emerald
    shards: [1, 2],
    apiPort: 2528,
    raftPort: 7003,
    term: 0,
    status: "Healthy",
  },
  {
    id: "node-e",
    name: "Node E",
    role: "Follower",
    pos: [1.7, -1.9, 0.3],
    color: "#7965E8",
    shards: [1],
    apiPort: 2529,
    raftPort: 7004,
    term: 0,
    status: "Healthy",
  },
];

// Shard configurations with targeted colors
const SHARD_CONFIGS = [
  { id: 0, name: "Shard 0", color: "#0891D5", replicas: ["node-a", "node-b", "node-c"] },
  { id: 1, name: "Shard 1", color: "#7965E8", replicas: ["node-d", "node-e", "node-a"] },
  { id: 2, name: "Shard 2", color: "#10B981", replicas: ["node-b", "node-c", "node-d"] },
];

export default function ClusterVisualization3D() {
  const mountRef = useRef<HTMLDivElement>(null);
  const [activeShard, setActiveShard] = useState<number | null>(null);
  const [nodes, setNodes] = useState<NodeData[]>(NODES);
  const [selectedNode, setSelectedNode] = useState<NodeData>(NODES[0]);
  const [hoveredNode, setHoveredNode] = useState<NodeData | null>(null);
  const [isLive, setIsLive] = useState<boolean>(false);
  const [liveEndpoint, setLiveEndpoint] = useState<string | null>(null);
  const [liveHealth, setLiveHealth] = useState<ClusterHealthResponse | null>(null);

  // Probe live backend on mount to discover actual cluster topology
  useEffect(() => {
    let mounted = true;
    const probeCluster = async () => {
      try {
        const res = await fetchClusterHealth({ timeoutMs: 1200 });
        if (!mounted) return;
        if (res.isLive && res.data) {
          setIsLive(true);
          setLiveEndpoint(res.endpoint);
          setLiveHealth(res.data);

          // Update nodes with actual roles and terms
          setNodes((prevNodes) =>
            prevNodes.map((n) => {
              const nodeStatus = res.data?.nodes?.[n.id];
              const isOnline = nodeStatus?.online ?? false;
              // Check if node is a leader in any shard
              let isLeader = false;
              let highestTerm = 0;
              if (res.data?.shards) {
                Object.values(res.data.shards).forEach((s) => {
                  if (s.current_leader === n.id) {
                    isLeader = true;
                  }
                  if (s.replica_node_ids?.includes(n.id) && s.current_term > highestTerm) {
                    highestTerm = s.current_term;
                  }
                });
              }

              return {
                ...n,
                role: isLeader ? "Leader" : "Follower",
                term: highestTerm,
                status: !isOnline ? "Offline" : isLeader ? "Elected" : "Healthy",
              };
            })
          );
        } else {
          setIsLive(false);
        }
      } catch {
        if (mounted) setIsLive(false);
      }
    };

    probeCluster();
    const interval = setInterval(probeCluster, 5000);
    return () => {
      mounted = false;
      clearInterval(interval);
    };
  }, []);

  useEffect(() => {
    const container = mountRef.current;
    if (!container) return;

    // --- Scene Setup ---
    const scene = new THREE.Scene();
    scene.fog = new THREE.FogExp2(0xf4f7fc, 0.04);

    const camera = new THREE.PerspectiveCamera(
      45,
      container.clientWidth / container.clientHeight,
      0.1,
      1000
    );
    camera.position.set(0, 0, 7.4);

    let renderer: THREE.WebGLRenderer;
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
      renderer.setSize(container.clientWidth, container.clientHeight);
      renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
      container.appendChild(renderer.domElement);
    } catch {
      return;
    }

    // --- Lighting for Rich 3D Sphere Appearance ---
    const ambientLight = new THREE.AmbientLight(0xffffff, 1.2);
    scene.add(ambientLight);

    const keyLight = new THREE.DirectionalLight(0xffffff, 2.0);
    keyLight.position.set(5, 8, 10);
    scene.add(keyLight);

    const cyanPointLight = new THREE.PointLight(0x0891d5, 3.0, 20);
    cyanPointLight.position.set(-4, 3, 5);
    scene.add(cyanPointLight);

    const purplePointLight = new THREE.PointLight(0x7965e8, 3.0, 20);
    purplePointLight.position.set(4, -3, 4);
    scene.add(purplePointLight);

    // Subtle background mesh particle field in light blue
    const particleCount = 140;
    const particleGeometry = new THREE.BufferGeometry();
    const positions = new Float32Array(particleCount * 3);
    for (let i = 0; i < particleCount * 3; i += 3) {
      positions[i] = (Math.random() - 0.5) * 16;
      positions[i + 1] = (Math.random() - 0.5) * 12;
      positions[i + 2] = (Math.random() - 0.5) * 6 - 2;
    }
    particleGeometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
    const particleMaterial = new THREE.PointsMaterial({
      color: 0x0891d5,
      size: 0.035,
      transparent: true,
      opacity: 0.25,
    });
    const starField = new THREE.Points(particleGeometry, particleMaterial);
    scene.add(starField);

    // --- Node Meshes with 3D Sphere Appearance & Shadows ---
    const nodeGroup = new THREE.Group();
    scene.add(nodeGroup);

    const nodeMeshes: { [key: string]: THREE.Mesh } = {};
    const shadowMeshes: { [key: string]: THREE.Mesh } = {};
    const ringMeshes: { [key: string]: THREE.Mesh } = {};

    NODES.forEach((node) => {
      const isLeader = node.role === "Leader";

      // 1. Core tactile 3D Sphere with specular gloss
      const sphereGeo = new THREE.SphereGeometry(0.42, 36, 36);
      const sphereMat = new THREE.MeshStandardMaterial({
        color: new THREE.Color(node.color),
        roughness: 0.15,
        metalness: 0.35,
        emissive: new THREE.Color(node.color),
        emissiveIntensity: 0.25,
      });
      const mesh = new THREE.Mesh(sphereGeo, sphereMat);
      mesh.position.set(node.pos[0], node.pos[1], node.pos[2]);
      nodeGroup.add(mesh);
      nodeMeshes[node.id] = mesh;

      // 2. Subtle drop shadow beneath node
      const shadowGeo = new THREE.CircleGeometry(0.38, 32);
      const shadowMat = new THREE.MeshBasicMaterial({
        color: 0x0f172a,
        transparent: true,
        opacity: 0.08,
      });
      const shadow = new THREE.Mesh(shadowGeo, shadowMat);
      shadow.position.set(node.pos[0], node.pos[1] - 0.6, node.pos[2]);
      shadow.rotation.x = -Math.PI / 2;
      nodeGroup.add(shadow);
      shadowMeshes[node.id] = shadow;

      // 3. Subtle Leader orbit ring
      if (isLeader) {
        const ringGeo = new THREE.RingGeometry(0.52, 0.56, 32);
        const ringMat = new THREE.MeshBasicMaterial({
          color: 0xf59e0b, // Amber for leader
          side: THREE.DoubleSide,
          transparent: true,
          opacity: 0.6,
        });
        const ring = new THREE.Mesh(ringGeo, ringMat);
        ring.position.set(node.pos[0], node.pos[1], node.pos[2]);
        nodeGroup.add(ring);
        ringMeshes[node.id] = ring;
      }
    });

    // --- Shard Colored Connection Lines ---
    interface LineData {
      shard: number;
      from: string;
      to: string;
      line: THREE.Line;
      color: string;
      curve: THREE.LineCurve3;
    }
    const lines: LineData[] = [];

    SHARD_CONFIGS.forEach((shard) => {
      const reps = shard.replicas;
      for (let i = 0; i < reps.length; i++) {
        for (let j = i + 1; j < reps.length; j++) {
          const n1 = NODES.find((n) => n.id === reps[i])!;
          const n2 = NODES.find((n) => n.id === reps[j])!;

          const p1 = new THREE.Vector3(n1.pos[0], n1.pos[1], n1.pos[2]);
          const p2 = new THREE.Vector3(n2.pos[0], n2.pos[1], n2.pos[2]);
          const curve = new THREE.LineCurve3(p1, p2);

          const geo = new THREE.BufferGeometry().setFromPoints(curve.getPoints(24));
          const mat = new THREE.LineBasicMaterial({
            color: new THREE.Color(shard.color),
            transparent: true,
            opacity: 0.35,
            linewidth: 2,
          });
          const line = new THREE.Line(geo, mat);
          nodeGroup.add(line);

          lines.push({
            shard: shard.id,
            from: reps[i],
            to: reps[j],
            line,
            color: shard.color,
            curve,
          });
        }
      }
    });

    // --- Animated Traveling Data Packets (Photons) ---
    const photonCount = 12;
    const photonGeo = new THREE.SphereGeometry(0.065, 12, 12);
    const photonMeshes: {
      mesh: THREE.Mesh;
      lineIndex: number;
      progress: number;
      speed: number;
    }[] = [];

    for (let i = 0; i < photonCount; i++) {
      const lineIdx = i % lines.length;
      const photonMat = new THREE.MeshBasicMaterial({
        color: new THREE.Color(lines[lineIdx].color),
        transparent: true,
        opacity: 0.95,
      });
      const pMesh = new THREE.Mesh(photonGeo, photonMat);
      nodeGroup.add(pMesh);
      photonMeshes.push({
        mesh: pMesh,
        lineIndex: lineIdx,
        progress: Math.random(),
        speed: 0.0035 + Math.random() * 0.003,
      });
    }

    // --- Apply Shard Highlighting ---
    const applyShardHighlight = (selectedShard: number | null) => {
      NODES.forEach((n) => {
        const mesh = nodeMeshes[n.id];
        if (!mesh) return;

        const isHighlighted =
          selectedShard === null ||
          SHARD_CONFIGS.find((s) => s.id === selectedShard)?.replicas.includes(n.id);

        const mat = mesh.material as THREE.MeshStandardMaterial;
        mat.opacity = isHighlighted ? 1.0 : 0.22;
        mat.transparent = !isHighlighted;
        mesh.scale.setScalar(isHighlighted ? 1.06 : 0.92);

        const shadow = shadowMeshes[n.id];
        if (shadow) {
          (shadow.material as THREE.MeshBasicMaterial).opacity = isHighlighted ? 0.08 : 0.02;
        }
      });

      lines.forEach((ld) => {
        const isLineActive = selectedShard === null || ld.shard === selectedShard;
        const mat = ld.line.material as THREE.LineBasicMaterial;
        mat.opacity = isLineActive ? (selectedShard === null ? 0.45 : 0.9) : 0.08;
      });

      photonMeshes.forEach((pm) => {
        const ld = lines[pm.lineIndex];
        const isPhotonActive = selectedShard === null || ld.shard === selectedShard;
        (pm.mesh.material as THREE.MeshBasicMaterial).opacity = isPhotonActive ? 0.95 : 0.1;
      });
    };

    applyShardHighlight(activeShard);

    // --- Mouse Parallax & Raycast Interaction ---
    let mouseX = 0;
    let mouseY = 0;
    let targetRotX = 0;
    let targetRotY = 0;

    // Raycast lookup helper using current dynamic node data
    const findNodeData = (meshObj: THREE.Object3D) => {
      const entry = Object.entries(nodeMeshes).find(([, mesh]) => mesh === meshObj);
      if (!entry) return null;
      return nodes.find((n) => n.id === entry[0]) || null;
    };

    const handleMouseMove = (e: MouseEvent) => {
      const rect = container.getBoundingClientRect();
      mouseX = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      mouseY = -(((e.clientY - rect.top) / rect.height) * 2 - 1);
      targetRotY = mouseX * 0.3;
      targetRotX = -mouseY * 0.2;

      // Raycasting for hover inspection
      const raycaster = new THREE.Raycaster();
      raycaster.setFromCamera(new THREE.Vector2(mouseX, mouseY), camera);
      const intersects = raycaster.intersectObjects(Object.values(nodeMeshes));
      if (intersects.length > 0) {
        const found = findNodeData(intersects[0].object);
        if (found) {
          setHoveredNode(found);
          document.body.style.cursor = "pointer";
          return;
        }
      }
      setHoveredNode(null);
      document.body.style.cursor = "default";
    };

    const handleClick = (e: MouseEvent) => {
      const rect = container.getBoundingClientRect();
      const mx = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      const my = -(((e.clientY - rect.top) / rect.height) * 2 - 1);
      const raycaster = new THREE.Raycaster();
      raycaster.setFromCamera(new THREE.Vector2(mx, my), camera);
      const intersects = raycaster.intersectObjects(Object.values(nodeMeshes));
      if (intersects.length > 0) {
        const found = findNodeData(intersects[0].object);
        if (found) {
          setSelectedNode(found);
        }
      }
    };

    window.addEventListener("mousemove", handleMouseMove);
    container.addEventListener("click", handleClick);

    // --- Animation Loop ---
    let animationFrameId: number;
    let clock = new THREE.Clock();

    const animate = () => {
      animationFrameId = requestAnimationFrame(animate);
      const delta = clock.getDelta();
      const elapsedTime = clock.getElapsedTime();

      // Smooth camera / nodeGroup rotation
      nodeGroup.rotation.y += (targetRotY - nodeGroup.rotation.y) * 0.05;
      nodeGroup.rotation.x += (targetRotX - nodeGroup.rotation.x) * 0.05;

      // Gentle floating animation
      nodeGroup.position.y = Math.sin(elapsedTime * 0.8) * 0.08;

      // Rotate leader rings
      Object.values(ringMeshes).forEach((r) => {
        r.rotation.z += 0.015;
      });

      // Animate data packets
      photonMeshes.forEach((pm) => {
        pm.progress += pm.speed;
        if (pm.progress > 1) pm.progress = 0;
        const line = lines[pm.lineIndex];
        const pt = line.curve.getPoint(pm.progress);
        pm.mesh.position.copy(pt);
      });

      renderer.render(scene, camera);
    };

    animate();

    const handleResize = () => {
      if (!container) return;
      camera.aspect = container.clientWidth / container.clientHeight;
      camera.updateProjectionMatrix();
      renderer.setSize(container.clientWidth, container.clientHeight);
    };

    window.addEventListener("resize", handleResize);

    return () => {
      window.removeEventListener("mousemove", handleMouseMove);
      container.removeEventListener("click", handleClick);
      window.removeEventListener("resize", handleResize);
      cancelAnimationFrame(animationFrameId);
      if (renderer.domElement && container.contains(renderer.domElement)) {
        container.removeChild(renderer.domElement);
      }
      renderer.dispose();
      document.body.style.cursor = "default";
    };
  }, [activeShard]);

  const activeDisplayNode = hoveredNode || selectedNode;

  return (
    <div className="relative w-full h-[500px] sm:h-[580px] flex items-center justify-center select-none bg-gradient-to-b from-[#F4F7FC] via-white to-[#EAF1FA] rounded-2xl border border-[#DCE5F0] shadow-[0_10px_35px_rgba(8,145,213,0.06)] overflow-hidden">
      {/* Subtle radial light glow backdrop */}
      <div className="absolute inset-0 bg-radial-light pointer-events-none opacity-60" />

      {/* 3D WebGL Canvas */}
      <div
        ref={mountRef}
        className="w-full h-full relative cursor-grab active:cursor-grabbing z-0"
      />

      {/* Shard Filter Selector Tabs (Top Overlay) */}
      <div className="absolute top-4 left-4 right-4 flex flex-wrap items-center justify-between gap-2 pointer-events-auto z-10">
        <div className="flex items-center gap-1.5 p-1 bg-white/90 backdrop-blur-md rounded-xl border border-[#DCE5F0] shadow-sm text-xs font-mono">
          <button
            onClick={() => setActiveShard(null)}
            className={`px-3 py-1 rounded-lg transition-all font-semibold ${
              activeShard === null
                ? "bg-[#0891D5]/15 text-[#0891D5] border border-[#0891D5]/30 shadow-sm"
                : "text-[#64748B] hover:text-[#17243A]"
            }`}
          >
            All Shards
          </button>
          {SHARD_CONFIGS.map((s) => {
            const isSelected = activeShard === s.id;
            return (
              <button
                key={s.id}
                onClick={() => setActiveShard(isSelected ? null : s.id)}
                className={`flex items-center gap-1.5 px-3 py-1 rounded-lg transition-all font-semibold ${
                  isSelected
                    ? "bg-slate-100 border text-[#17243A] shadow-sm"
                    : "text-[#64748B] hover:text-[#17243A]"
                }`}
                style={{
                  borderColor: isSelected ? s.color : "transparent",
                }}
              >
                <span
                  className="w-2 h-2 rounded-full"
                  style={{ backgroundColor: s.color }}
                />
                <span>{s.name}</span>
              </button>
            );
          })}
        </div>

        {/* Live Indicator */}
        {isLive ? (
          <div className="hidden sm:flex items-center gap-2 px-3 py-1.5 bg-emerald-50/95 backdrop-blur-md rounded-xl border border-emerald-200 text-xs font-mono text-emerald-800 shadow-sm">
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
            <span>Live Cluster Connected ({liveEndpoint || ":2525"})</span>
          </div>
        ) : (
          <div className="hidden sm:flex items-center gap-2 px-3 py-1.5 bg-white/90 backdrop-blur-md rounded-xl border border-[#DCE5F0] text-xs font-mono text-[#64748B] shadow-sm">
            <Radio className="w-3.5 h-3.5 text-slate-400" />
            <span>Architectural Simulation (Cluster Offline)</span>
          </div>
        )}
      </div>

      {/* Node Inspector Overlay Card (Bottom Left Overlay) */}
      <div className="absolute bottom-4 left-4 right-4 sm:left-4 sm:right-auto sm:w-80 p-3.5 bg-white/95 backdrop-blur-md rounded-xl border border-[#DCE5F0] shadow-lg shadow-slate-200/50 pointer-events-auto transition-all z-10">
        <div className="flex items-center justify-between pb-2 mb-2 border-b border-[#DCE5F0]">
          <div className="flex items-center gap-2">
            <span
              className="w-3 h-3 rounded-full"
              style={{ backgroundColor: activeDisplayNode.color }}
            />
            <span className="font-bold text-sm text-[#17243A] font-mono">
              {activeDisplayNode.name}
            </span>
            <span className="text-[10px] text-[#64748B] font-mono">
              ({activeDisplayNode.id})
            </span>
          </div>
          <span
            className={`text-[10px] font-mono px-2 py-0.5 rounded-full font-semibold uppercase tracking-wider ${
              activeDisplayNode.status === "Offline"
                ? "bg-slate-100 text-slate-600 border border-slate-200"
                : activeDisplayNode.role === "Leader"
                ? "bg-amber-50 text-amber-700 border border-amber-200 flex items-center gap-1"
                : "bg-sky-50 text-[#0891D5] border border-sky-200"
            }`}
          >
            {activeDisplayNode.role === "Leader" && activeDisplayNode.status !== "Offline" && <Crown className="w-2.5 h-2.5" />}
            {activeDisplayNode.status === "Offline" ? "OFFLINE" : activeDisplayNode.role}
          </span>
        </div>

        <div className="grid grid-cols-2 gap-2 text-xs font-mono mb-2">
          <div className="bg-[#F4F7FC] p-2 rounded-lg border border-[#DCE5F0]">
            <span className="text-[10px] text-[#64748B] block">HTTP API</span>
            <span className="text-[#0891D5] font-semibold">
              127.0.0.1:{activeDisplayNode.apiPort}
            </span>
          </div>
          <div className="bg-[#F4F7FC] p-2 rounded-lg border border-[#DCE5F0]">
            <span className="text-[10px] text-[#64748B] block">Raft Port</span>
            <span className="text-[#7965E8] font-semibold">
              127.0.0.1:{activeDisplayNode.raftPort}
            </span>
          </div>
        </div>

        <div className="flex items-center justify-between text-[11px] font-mono text-[#17243A] bg-[#F4F7FC] px-2.5 py-1.5 rounded-lg border border-[#DCE5F0]">
          <div className="flex items-center gap-1.5">
            <Layers className="w-3.5 h-3.5 text-[#0891D5]" />
            <span className="text-[#64748B]">Assigned Shards:</span>
          </div>
          <div className="flex gap-1">
            {activeDisplayNode.shards.map((sid) => (
              <span
                key={sid}
                className="px-1.5 py-0.2 rounded bg-white border border-[#DCE5F0] text-[#17243A] font-bold text-[10px]"
              >
                Shard {sid}
              </span>
            ))}
          </div>
        </div>

        <p className="mt-2 text-[10px] text-[#64748B] font-mono text-center">
          Hover or click physical nodes in 3D space to inspect consensus telemetry.
        </p>
      </div>

      {/* Shard Placement Reference Matrix (Bottom Right Overlay) */}
      <div className="hidden lg:block absolute bottom-4 right-4 p-3 bg-white/95 backdrop-blur-md rounded-xl border border-[#DCE5F0] text-xs font-mono text-[#17243A] shadow-md pointer-events-auto z-10">
        <span className="text-[10px] uppercase tracking-wider text-[#64748B] font-bold block mb-1.5">
          Replica Placement Matrix {isLive ? "(Live)" : "(Configured)"}
        </span>
        <div className="space-y-1 text-[11px]">
          {SHARD_CONFIGS.map((s) => {
            const liveLeader = liveHealth?.shards?.[s.id]?.current_leader;
            return (
              <div key={s.id} className="flex items-center justify-between gap-4">
                <span style={{ color: s.color }} className="font-semibold">
                  Shard {s.id}:
                </span>
                <span className="text-[#17243A]">
                  {s.replicas
                    .map((r) => {
                      const nodeName = r.replace("node-", "Node ").toUpperCase();
                      const isLeaderNode = liveLeader ? r === liveLeader : r === s.replicas[0];
                      return isLeaderNode ? `${nodeName} (L)` : nodeName;
                    })
                    .join(", ")}
                </span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
