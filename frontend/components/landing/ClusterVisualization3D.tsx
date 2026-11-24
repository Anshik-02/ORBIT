"use client";

import React, { useEffect, useRef, useState } from "react";
import * as THREE from "three";
import { ShieldCheck, Activity, Layers, Radio, Sparkles, Server } from "lucide-react";

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
  status: "Healthy" | "Replicating" | "Elected";
}

const NODES: NodeData[] = [
  {
    id: "node-a",
    name: "Node A",
    role: "Leader",
    pos: [0, 2.0, 0.2],
    color: "#38bdf8", // Sky blue
    shards: [0, 1], // Shard 0 leader, Shard 1 replica
    apiPort: 8000,
    raftPort: 9000,
    term: 4,
    status: "Healthy",
  },
  {
    id: "node-b",
    name: "Node B",
    role: "Leader",
    pos: [-2.4, 0.7, 0.4],
    color: "#818cf8", // Indigo
    shards: [0, 2], // Shard 0 replica, Shard 2 leader
    apiPort: 8001,
    raftPort: 9001,
    term: 4,
    status: "Healthy",
  },
  {
    id: "node-c",
    name: "Node C",
    role: "Follower",
    pos: [2.4, 0.7, -0.4],
    color: "#38bdf8",
    shards: [0, 2], // Shard 0 replica, Shard 2 replica
    apiPort: 8002,
    raftPort: 9002,
    term: 4,
    status: "Healthy",
  },
  {
    id: "node-d",
    name: "Node D",
    role: "Leader",
    pos: [-1.6, -1.8, -0.3],
    color: "#34d399", // Emerald
    shards: [1, 2], // Shard 1 leader, Shard 2 replica
    apiPort: 8003,
    raftPort: 9003,
    term: 4,
    status: "Healthy",
  },
  {
    id: "node-e",
    name: "Node E",
    role: "Follower",
    pos: [1.6, -1.8, 0.3],
    color: "#c084fc", // Purple
    shards: [1], // Shard 1 replica
    apiPort: 8004,
    raftPort: 9004,
    term: 4,
    status: "Healthy",
  },
];

// Replicas connections by shard
// Shard 0: A, B, C
// Shard 1: D, E, A
// Shard 2: B, C, D
const SHARD_CONFIGS = [
  { id: 0, name: "Shard 0", color: "#38bdf8", replicas: ["node-a", "node-b", "node-c"] },
  { id: 1, name: "Shard 1", color: "#c084fc", replicas: ["node-d", "node-e", "node-a"] },
  { id: 2, name: "Shard 2", color: "#34d399", replicas: ["node-b", "node-c", "node-d"] },
];

export default function ClusterVisualization3D() {
  const mountRef = useRef<HTMLDivElement>(null);
  const [activeShard, setActiveShard] = useState<number | null>(null);
  const [selectedNode, setSelectedNode] = useState<NodeData>(NODES[0]);
  const [hoveredNode, setHoveredNode] = useState<NodeData | null>(null);

  useEffect(() => {
    const container = mountRef.current;
    if (!container) return;

    // --- Scene Setup ---
    const scene = new THREE.Scene();
    scene.fog = new THREE.FogExp2(0x030712, 0.05);

    const camera = new THREE.PerspectiveCamera(
      45,
      container.clientWidth / container.clientHeight,
      0.1,
      1000
    );
    camera.position.set(0, 0, 7.2);

    let renderer: THREE.WebGLRenderer;
    try {
      renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
      renderer.setSize(container.clientWidth, container.clientHeight);
      renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
      container.appendChild(renderer.domElement);
    } catch {
      return;
    }

    // --- Lighting ---
    const ambientLight = new THREE.AmbientLight(0xffffff, 0.7);
    scene.add(ambientLight);

    const pointLight = new THREE.PointLight(0x38bdf8, 2.5, 50);
    pointLight.position.set(0, 5, 8);
    scene.add(pointLight);

    const blueLight = new THREE.PointLight(0x818cf8, 2, 30);
    blueLight.position.set(-5, -3, 2);
    scene.add(blueLight);

    // --- Background Star / Particle Field ---
    const particleCount = 220;
    const particleGeometry = new THREE.BufferGeometry();
    const positions = new Float32Array(particleCount * 3);
    for (let i = 0; i < particleCount * 3; i += 3) {
      positions[i] = (Math.random() - 0.5) * 16;
      positions[i + 1] = (Math.random() - 0.5) * 12;
      positions[i + 2] = (Math.random() - 0.5) * 8 - 2;
    }
    particleGeometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
    const particleMaterial = new THREE.PointsMaterial({
      color: 0x38bdf8,
      size: 0.04,
      transparent: true,
      opacity: 0.45,
    });
    const starField = new THREE.Points(particleGeometry, particleMaterial);
    scene.add(starField);

    // --- Node Meshes ---
    const nodeGroup = new THREE.Group();
    scene.add(nodeGroup);

    const nodeMeshes: { [key: string]: THREE.Mesh } = {};
    const haloMeshes: { [key: string]: THREE.Mesh } = {};

    NODES.forEach((node) => {
      // Core sphere
      const sphereGeo = new THREE.IcosahedronGeometry(0.38, 2);
      const sphereMat = new THREE.MeshStandardMaterial({
        color: new THREE.Color(node.color),
        emissive: new THREE.Color(node.color),
        emissiveIntensity: node.role === "Leader" ? 0.7 : 0.35,
        roughness: 0.2,
        metalness: 0.8,
      });
      const mesh = new THREE.Mesh(sphereGeo, sphereMat);
      mesh.position.set(...node.pos);
      nodeGroup.add(mesh);
      nodeMeshes[node.id] = mesh;

      // Outer glow ring / wireframe
      const haloGeo = new THREE.RingGeometry(0.48, 0.54, 32);
      const haloMat = new THREE.MeshBasicMaterial({
        color: new THREE.Color(node.color),
        side: THREE.DoubleSide,
        transparent: true,
        opacity: node.role === "Leader" ? 0.75 : 0.3,
      });
      const halo = new THREE.Mesh(haloGeo, haloMat);
      halo.position.set(...node.pos);
      nodeGroup.add(halo);
      haloMeshes[node.id] = halo;
    });

    // --- Dynamic Connection Lines & Photons ---
    // Connections exist for each pair in a shard
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

          const p1 = new THREE.Vector3(...n1.pos);
          const p2 = new THREE.Vector3(...n2.pos);
          const curve = new THREE.LineCurve3(p1, p2);

          const geo = new THREE.BufferGeometry().setFromPoints(curve.getPoints(24));
          const mat = new THREE.LineBasicMaterial({
            color: new THREE.Color(shard.color),
            transparent: true,
            opacity: 0.25,
            linewidth: 1,
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

    // Traveling Data Packets (Photons)
    const photonCount = 14;
    const photonGeo = new THREE.SphereGeometry(0.06, 8, 8);
    const photonMeshes: { mesh: THREE.Mesh; lineIndex: number; progress: number; speed: number }[] = [];

    for (let i = 0; i < photonCount; i++) {
      const lineIdx = i % lines.length;
      const photonMat = new THREE.MeshBasicMaterial({
        color: new THREE.Color(lines[lineIdx].color),
        transparent: true,
        opacity: 0.9,
      });
      const pMesh = new THREE.Mesh(photonGeo, photonMat);
      nodeGroup.add(pMesh);
      photonMeshes.push({
        mesh: pMesh,
        lineIndex: lineIdx,
        progress: Math.random(),
        speed: 0.003 + Math.random() * 0.004,
      });
    }

    // --- Mouse Parallax & Interaction ---
    let mouseX = 0;
    let mouseY = 0;
    let targetRotX = 0;
    let targetRotY = 0;

    const handleMouseMove = (e: MouseEvent) => {
      const rect = container.getBoundingClientRect();
      mouseX = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      mouseY = -(((e.clientY - rect.top) / rect.height) * 2 - 1);
      targetRotY = mouseX * 0.35;
      targetRotX = -mouseY * 0.25;

      // Raycasting for hover
      const raycaster = new THREE.Raycaster();
      const mouseVec = new THREE.Vector2(mouseX, mouseY);
      raycaster.setFromCamera(mouseVec, camera);
      const intersects = raycaster.intersectObjects(Object.values(nodeMeshes));
      if (intersects.length > 0) {
        const hit = intersects[0].object;
        const found = NODES.find((n) => nodeMeshes[n.id] === hit);
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
        const hit = intersects[0].object;
        const found = NODES.find((n) => nodeMeshes[n.id] === hit);
        if (found) {
          setSelectedNode(found);
        }
      }
    };

    container.addEventListener("mousemove", handleMouseMove);
    container.addEventListener("click", handleClick);

    // --- Animation Loop ---
    let animationFrameId: number;
    let clock = new THREE.Clock();

    const animate = () => {
      animationFrameId = requestAnimationFrame(animate);
      const elapsedTime = clock.getElapsedTime();

      // Smooth camera / group rotation
      nodeGroup.rotation.y += (targetRotY - nodeGroup.rotation.y) * 0.05;
      nodeGroup.rotation.x += (targetRotX - nodeGroup.rotation.x) * 0.05;

      // Gentle cluster breathing
      nodeGroup.position.y = Math.sin(elapsedTime * 0.8) * 0.08;

      // Rotate starfield slowly
      starField.rotation.y = elapsedTime * 0.02;

      // Pulsing halos
      NODES.forEach((n) => {
        const halo = haloMeshes[n.id];
        if (halo) {
          halo.lookAt(camera.position);
          const scale = 1 + Math.sin(elapsedTime * 3 + (n.role === "Leader" ? 0 : 2)) * 0.08;
          halo.scale.set(scale, scale, scale);
        }
      });

      // Update line highlights based on activeShard
      lines.forEach((l) => {
        const isDim = activeShard !== null && l.shard !== activeShard;
        const mat = l.line.material as THREE.LineBasicMaterial;
        mat.opacity = isDim ? 0.06 : 0.35;
      });

      // Animate Photons
      photonMeshes.forEach((p) => {
        const l = lines[p.lineIndex];
        const isDim = activeShard !== null && l.shard !== activeShard;
        p.mesh.visible = !isDim;

        p.progress += p.speed;
        if (p.progress > 1) {
          p.progress = 0;
          p.lineIndex = Math.floor(Math.random() * lines.length);
        }
        const point = l.curve.getPoint(p.progress);
        p.mesh.position.copy(point);
      });

      renderer.render(scene, camera);
    };

    animate();

    // --- Resize Handler ---
    const handleResize = () => {
      if (!container) return;
      const w = container.clientWidth;
      const h = container.clientHeight;
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
      renderer.setSize(w, h);
    };

    window.addEventListener("resize", handleResize);

    return () => {
      window.removeEventListener("resize", handleResize);
      container.removeEventListener("mousemove", handleMouseMove);
      container.removeEventListener("click", handleClick);
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
    <div className="relative w-full h-[520px] sm:h-[600px] flex items-center justify-center select-none">
      {/* 3D WebGL Canvas Container */}
      <div
        ref={mountRef}
        className="w-full h-full rounded-2xl overflow-hidden relative cursor-grab active:cursor-grabbing"
      />

      {/* Shard Filter Selector Tabs (Top Overlay) */}
      <div className="absolute top-4 left-4 right-4 flex flex-wrap items-center justify-between gap-2 pointer-events-auto">
        <div className="flex items-center gap-1.5 p-1 bg-slate-950/80 backdrop-blur-md rounded-xl border border-sky-500/20 shadow-lg text-xs font-mono">
          <button
            onClick={() => setActiveShard(null)}
            className={`px-2.5 py-1 rounded-lg transition-all ${
              activeShard === null
                ? "bg-sky-500/30 text-sky-200 border border-sky-400/40 shadow-sm"
                : "text-slate-400 hover:text-slate-200"
            }`}
          >
            All Shards
          </button>
          {SHARD_CONFIGS.map((s) => (
            <button
              key={s.id}
              onClick={() => setActiveShard(activeShard === s.id ? null : s.id)}
              className={`flex items-center gap-1.5 px-2.5 py-1 rounded-lg transition-all ${
                activeShard === s.id
                  ? "bg-slate-800 text-white border border-slate-600 shadow-sm"
                  : "text-slate-400 hover:text-slate-200"
              }`}
            >
              <span
                className="w-2 h-2 rounded-full"
                style={{ backgroundColor: s.color }}
              />
              <span>{s.name}</span>
            </button>
          ))}
        </div>

        {/* Live Topology Indicator */}
        <div className="hidden sm:flex items-center gap-2 px-3 py-1.5 bg-slate-950/80 backdrop-blur-md rounded-xl border border-sky-500/20 text-xs font-mono text-slate-300">
          <Radio className="w-3.5 h-3.5 text-emerald-400 animate-pulse" />
          <span>Interactive 3D Cluster</span>
        </div>
      </div>

      {/* Node Inspector Overlay Card (Bottom Overlay) */}
      <div className="absolute bottom-4 left-4 right-4 sm:left-4 sm:right-auto sm:w-80 p-3.5 glass-panel rounded-xl border border-sky-500/25 shadow-xl shadow-black/60 pointer-events-auto transition-all animate-fadeIn">
        <div className="flex items-center justify-between pb-2 mb-2 border-b border-slate-800/80">
          <div className="flex items-center gap-2">
            <div
              className="w-3 h-3 rounded-full animate-ping"
              style={{ backgroundColor: activeDisplayNode.color }}
            />
            <span className="font-bold text-sm text-white font-mono">
              {activeDisplayNode.name}
            </span>
            <span className="text-[10px] text-slate-400 font-mono">({activeDisplayNode.id})</span>
          </div>
          <span
            className={`text-[10px] font-mono px-2 py-0.5 rounded-full font-semibold uppercase tracking-wider ${
              activeDisplayNode.role === "Leader"
                ? "bg-amber-500/20 text-amber-300 border border-amber-500/40"
                : "bg-sky-500/20 text-sky-300 border border-sky-500/40"
            }`}
          >
            {activeDisplayNode.role}
          </span>
        </div>

        <div className="grid grid-cols-2 gap-2 text-xs font-mono mb-2">
          <div className="bg-slate-900/60 p-2 rounded-lg border border-slate-800">
            <span className="text-[10px] text-slate-400 block">HTTP Client API</span>
            <span className="text-sky-300 font-semibold">127.0.0.1:{activeDisplayNode.apiPort}</span>
          </div>
          <div className="bg-slate-900/60 p-2 rounded-lg border border-slate-800">
            <span className="text-[10px] text-slate-400 block">Raft Transport</span>
            <span className="text-purple-300 font-semibold">127.0.0.1:{activeDisplayNode.raftPort}</span>
          </div>
        </div>

        <div className="flex items-center justify-between text-[11px] font-mono text-slate-300 bg-slate-900/40 px-2.5 py-1.5 rounded-lg border border-slate-800/80">
          <div className="flex items-center gap-1.5">
            <Layers className="w-3.5 h-3.5 text-sky-400" />
            <span>Hosted Shards:</span>
          </div>
          <div className="flex gap-1">
            {activeDisplayNode.shards.map((sid) => (
              <span
                key={sid}
                className="px-1.5 py-0.2 rounded bg-sky-950 border border-sky-500/40 text-sky-300 font-bold"
              >
                Shard {sid}
              </span>
            ))}
          </div>
        </div>

        <p className="mt-2 text-[10px] text-slate-400 font-mono text-center">
          Hover or click physical nodes in 3D space to inspect consensus telemetry.
        </p>
      </div>

      {/* Shard Replica Placement Reference (Bottom Right Badge) */}
      <div className="hidden lg:block absolute bottom-4 right-4 p-3 glass-panel rounded-xl border border-sky-500/20 text-xs font-mono text-slate-300 pointer-events-auto">
        <span className="text-[10px] uppercase tracking-wider text-slate-400 font-bold block mb-1.5">
          Replica Placement Matrix
        </span>
        <div className="space-y-1 text-[11px]">
          <div className="flex items-center justify-between gap-4">
            <span className="text-sky-400 font-semibold">Shard 0:</span>
            <span className="text-slate-300">Node A (L), Node B, Node C</span>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="text-purple-400 font-semibold">Shard 1:</span>
            <span className="text-slate-300">Node D (L), Node E, Node A</span>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="text-emerald-400 font-semibold">Shard 2:</span>
            <span className="text-slate-300">Node B (L), Node C, Node D</span>
          </div>
        </div>
      </div>
    </div>
  );
}
