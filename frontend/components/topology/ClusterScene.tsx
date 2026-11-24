'use client';

import React, { useRef, useEffect, useState, useCallback } from 'react';
import * as THREE from 'three';
import {
  PhysicalNode,
  NetworkLink,
  ViewMode,
  NetworkMode,
} from '../../lib/topology/types';
import { CameraPreset } from './TopologyControls';

interface ClusterSceneProps {
  nodes: PhysicalNode[];
  links: NetworkLink[];
  viewMode: ViewMode;
  networkMode: NetworkMode;
  selectedNodeId: string | null;
  selectedShardId?: number | 'all';
  cameraPreset?: CameraPreset | null;
  onSelectNode: (nodeId: string | null) => void;
  hoveredLinkId: string | null;
  onHoverLink: (linkId: string | null) => void;
  onResetView?: () => void;
}

export default function ClusterScene({
  nodes,
  links,
  viewMode,
  networkMode,
  selectedNodeId,
  selectedShardId = 'all',
  cameraPreset,
  onSelectNode,
  hoveredLinkId,
  onHoverLink,
}: ClusterSceneProps) {
  const mountRef = useRef<HTMLDivElement>(null);
  const sceneRef = useRef<THREE.Scene | null>(null);
  const cameraRef = useRef<THREE.PerspectiveCamera | null>(null);
  const rendererRef = useRef<THREE.WebGLRenderer | null>(null);
  const buildingMeshesRef = useRef<Map<string, THREE.Group>>(new Map());
  const linkLinesRef = useRef<Map<string, { line: THREE.Line; curve: THREE.QuadraticBezierCurve3 }>>(new Map());
  const packetParticlesRef = useRef<
    Array<{ mesh: THREE.Mesh; curve: THREE.QuadraticBezierCurve3; progress: number; speed: number }>
  >([]);
  const leaderRingsRef = useRef<THREE.Mesh[]>([]);
  const animFrameIdRef = useRef<number | null>(null);

  // Orbit controls state
  const isDraggingRef = useRef(false);
  const previousMousePositionRef = useRef({ x: 0, y: 0 });
  const cameraAngleRef = useRef({ theta: Math.PI / 4, phi: Math.PI / 3, radius: 16 });

  // Tooltip state for link hover in 3D/2D
  const [hoveredLinkTooltip, setHoveredLinkTooltip] = useState<{
    link: NetworkLink;
    x: number;
    y: number;
  } | null>(null);

  // Helper to create node billboard label texture
  const createLabelTexture = (nodeName: string, port: string, isOnline: boolean, isLeader: boolean) => {
    const canvas = document.createElement('canvas');
    canvas.width = 256;
    canvas.height = 128;
    const ctx = canvas.getContext('2d');
    if (!ctx) return new THREE.CanvasTexture(canvas);

    // Background rounded pill
    ctx.fillStyle = isOnline ? 'rgba(255, 255, 255, 0.96)' : 'rgba(241, 245, 249, 0.96)';
    ctx.strokeStyle = isLeader ? '#F59E0B' : isOnline ? '#0891D5' : '#EF4444';
    ctx.lineWidth = 4;
    ctx.beginPath();
    ctx.roundRect(16, 20, 224, 88, 20);
    ctx.fill();
    ctx.stroke();

    // Node Name
    ctx.fillStyle = '#17243A';
    ctx.font = 'bold 32px sans-serif';
    ctx.textAlign = 'center';
    ctx.fillText(nodeName, 128, 62);

    // Port or Status
    ctx.fillStyle = isOnline ? '#64748B' : '#EF4444';
    ctx.font = 'bold 20px monospace';
    ctx.fillText(isOnline ? port : 'OFFLINE', 128, 92);

    const texture = new THREE.CanvasTexture(canvas);
    texture.minFilter = THREE.LinearFilter;
    return texture;
  };

  // Build a Stylized 3D Server Building
  const createServerBuilding = (node: PhysicalNode, isSelected: boolean) => {
    const group = new THREE.Group();
    group.name = node.id;
    group.position.set(node.position3D[0], node.position3D[1], node.position3D[2]);

    const isOnline = node.online;
    const isLeader = node.hostedShards.some((s) => s.role === 'Leader');

    // 1. Foundation Base Plinth
    const plinthGeo = new THREE.CylinderGeometry(1.4, 1.6, 0.3, 32);
    const plinthMat = new THREE.MeshStandardMaterial({
      color: isOnline ? 0x0891d5 : 0x94a3b8,
      metalness: 0.5,
      roughness: 0.3,
      transparent: true,
      opacity: 0.85,
    });
    const plinth = new THREE.Mesh(plinthGeo, plinthMat);
    plinth.position.y = 0.15;
    group.add(plinth);

    // Cyan Outline / Glow Ring when Selected
    if (isSelected) {
      const selectHaloGeo = new THREE.TorusGeometry(1.9, 0.08, 16, 64);
      const selectHaloMat = new THREE.MeshBasicMaterial({ color: 0x0891d5 });
      const selectHalo = new THREE.Mesh(selectHaloGeo, selectHaloMat);
      selectHalo.rotation.x = Math.PI / 2;
      selectHalo.position.y = 0.05;
      group.add(selectHalo);
    }

    // Leader Dual Amber Rings & Plinth Crown
    if (isLeader && isOnline) {
      const ringGeo = new THREE.TorusGeometry(1.7, 0.06, 16, 64);
      const ringMat = new THREE.MeshBasicMaterial({ color: 0xf59e0b });
      const ring = new THREE.Mesh(ringGeo, ringMat);
      ring.rotation.x = Math.PI / 2;
      ring.position.y = 0.12;
      group.add(ring);
      leaderRingsRef.current.push(ring);
    }

    // 2. Main Server Tower Chassis
    const towerWidth = 1.3;
    const towerHeight = 2.6;
    const towerDepth = 1.3;
    const towerGeo = new THREE.BoxGeometry(towerWidth, towerHeight, towerDepth);
    const towerMat = new THREE.MeshStandardMaterial({
      color: isOnline ? (isSelected ? 0x0f172a : 0x1e293b) : 0x475569,
      metalness: 0.8,
      roughness: 0.25,
    });
    const tower = new THREE.Mesh(towerGeo, towerMat);
    tower.position.y = towerHeight / 2 + 0.3;
    group.add(tower);

    // 3. Multi-Tiered Server Blades with Shard Indicators
    const bladeCount = 5;
    for (let i = 0; i < bladeCount; i++) {
      const yOffset = 0.6 + i * 0.42;
      const bladeGeo = new THREE.BoxGeometry(towerWidth + 0.02, 0.28, towerDepth + 0.02);
      const bladeMat = new THREE.MeshStandardMaterial({
        color: 0x0f172a,
        metalness: 0.9,
        roughness: 0.2,
      });
      const blade = new THREE.Mesh(bladeGeo, bladeMat);
      blade.position.y = yOffset;
      group.add(blade);

      // Status LED dots on front panel
      const ledColor = isOnline
        ? i === 0 && isLeader
          ? 0xf59e0b
          : 0x10b981
        : 0xef4444;

      const ledGeo = new THREE.SphereGeometry(0.04, 8, 8);
      const ledMat = new THREE.MeshBasicMaterial({ color: ledColor });

      const led1 = new THREE.Mesh(ledGeo, ledMat);
      led1.position.set(-0.45, yOffset, towerDepth / 2 + 0.02);
      group.add(led1);

      const led2 = new THREE.Mesh(ledGeo, ledMat);
      led2.position.set(-0.32, yOffset, towerDepth / 2 + 0.02);
      group.add(led2);

      // Shard-colored indicator strip on the side of each blade
      if (node.hostedShards[i]) {
        const sh = node.hostedShards[i];
        const shardColor =
          sh.shardId === 0 ? 0x0891d5 : sh.shardId === 1 ? 0x7965e8 : 0x10b981;
        const stripGeo = new THREE.BoxGeometry(0.02, 0.16, 0.6);
        const stripMat = new THREE.MeshBasicMaterial({ color: shardColor });
        const strip = new THREE.Mesh(stripGeo, stripMat);
        strip.position.set(towerWidth / 2 + 0.02, yOffset, 0);
        group.add(strip);
      }
    }

    // 4. Roof Ventilation / Antenna Beacon
    const beaconGeo = new THREE.CylinderGeometry(0.12, 0.2, 0.4, 16);
    const beaconMat = new THREE.MeshBasicMaterial({
      color: isOnline ? (isLeader ? 0xf59e0b : 0x0891d5) : 0xef4444,
    });
    const beacon = new THREE.Mesh(beaconGeo, beaconMat);
    beacon.position.y = towerHeight + 0.5;
    group.add(beacon);

    // 5. Billboard Label Above Tower
    const labelTexture = createLabelTexture(
      node.name,
      node.httpAddress.replace('127.0.0.1:', ':'),
      node.online,
      isLeader
    );
    const spriteMat = new THREE.SpriteMaterial({ map: labelTexture, transparent: true });
    const sprite = new THREE.Sprite(spriteMat);
    sprite.scale.set(2.2, 1.1, 1);
    sprite.position.y = towerHeight + 1.4;
    group.add(sprite);

    // Invisible Hit Box for Raycasting
    const hitBoxGeo = new THREE.BoxGeometry(2.4, 4.2, 2.4);
    const hitBoxMat = new THREE.MeshBasicMaterial({ visible: false });
    const hitBox = new THREE.Mesh(hitBoxGeo, hitBoxMat);
    hitBox.position.y = 2.1;
    hitBox.userData = { nodeId: node.id };
    group.add(hitBox);

    return group;
  };

  // Camera preset handler
  const applyCameraPreset = useCallback((preset: CameraPreset) => {
    if (!cameraRef.current) return;
    if (preset === 'isometric') {
      cameraAngleRef.current = { theta: Math.PI / 4, phi: Math.PI / 3, radius: 16 };
    } else if (preset === 'top') {
      cameraAngleRef.current = { theta: 0.001, phi: 0.1, radius: 20 };
    } else if (preset === 'front') {
      cameraAngleRef.current = { theta: 0, phi: Math.PI / 2.3, radius: 14 };
    } else if (preset === 'reset') {
      cameraAngleRef.current = { theta: Math.PI / 4, phi: Math.PI / 3, radius: 16 };
    }

    const { theta, phi, radius } = cameraAngleRef.current;
    cameraRef.current.position.x = radius * Math.sin(phi) * Math.sin(theta);
    cameraRef.current.position.y = radius * Math.cos(phi);
    cameraRef.current.position.z = radius * Math.sin(phi) * Math.cos(theta);
    cameraRef.current.lookAt(0, 0.5, 0);
  }, []);

  useEffect(() => {
    if (cameraPreset) {
      applyCameraPreset(cameraPreset);
    }
  }, [cameraPreset, applyCameraPreset]);

  // Initialize Three.js Scene
  useEffect(() => {
    if (!mountRef.current || viewMode !== '3d') return;

    const width = mountRef.current.clientWidth;
    const height = mountRef.current.clientHeight;

    const scene = new THREE.Scene();
    sceneRef.current = scene;
    scene.background = new THREE.Color(0xf4f7fc);
    scene.fog = new THREE.Fog(0xf4f7fc, 18, 42);

    const camera = new THREE.PerspectiveCamera(45, width / height, 0.1, 100);
    cameraRef.current = camera;

    const updateCameraPos = () => {
      const { theta, phi, radius } = cameraAngleRef.current;
      camera.position.x = radius * Math.sin(phi) * Math.sin(theta);
      camera.position.y = radius * Math.cos(phi);
      camera.position.z = radius * Math.sin(phi) * Math.cos(theta);
      camera.lookAt(0, 0.5, 0);
    };
    updateCameraPos();

    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
    rendererRef.current = renderer;
    renderer.setSize(width, height);
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    renderer.shadowMap.enabled = true;
    mountRef.current.appendChild(renderer.domElement);

    // Lighting
    const ambientLight = new THREE.AmbientLight(0xffffff, 1.4);
    scene.add(ambientLight);

    const dirLight1 = new THREE.DirectionalLight(0xffffff, 1.8);
    dirLight1.position.set(10, 20, 15);
    scene.add(dirLight1);

    const dirLight2 = new THREE.DirectionalLight(0x0891d5, 0.6);
    dirLight2.position.set(-15, 10, -10);
    scene.add(dirLight2);

    // Tech Grid Floor
    const gridHelper = new THREE.GridHelper(30, 30, 0x0891d5, 0xdce5f0);
    gridHelper.position.y = 0;
    scene.add(gridHelper);

    // Floor subtle circular disc
    const floorDiscGeo = new THREE.CircleGeometry(11, 64);
    const floorDiscMat = new THREE.MeshBasicMaterial({
      color: 0xffffff,
      transparent: true,
      opacity: 0.6,
    });
    const floorDisc = new THREE.Mesh(floorDiscGeo, floorDiscMat);
    floorDisc.rotation.x = -Math.PI / 2;
    floorDisc.position.y = -0.01;
    scene.add(floorDisc);

    // Mouse drag rotation handlers
    const dom = renderer.domElement;
    const onMouseDown = (e: MouseEvent) => {
      isDraggingRef.current = true;
      previousMousePositionRef.current = { x: e.clientX, y: e.clientY };
    };

    const onMouseMove = (e: MouseEvent) => {
      if (!isDraggingRef.current) return;
      const deltaX = e.clientX - previousMousePositionRef.current.x;
      const deltaY = e.clientY - previousMousePositionRef.current.y;

      cameraAngleRef.current.theta -= deltaX * 0.008;
      cameraAngleRef.current.phi = Math.max(
        0.05,
        Math.min(Math.PI / 2 - 0.05, cameraAngleRef.current.phi - deltaY * 0.008)
      );

      updateCameraPos();
      previousMousePositionRef.current = { x: e.clientX, y: e.clientY };
    };

    const onMouseUp = () => {
      isDraggingRef.current = false;
    };

    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      cameraAngleRef.current.radius = Math.max(
        8,
        Math.min(30, cameraAngleRef.current.radius + e.deltaY * 0.02)
      );
      updateCameraPos();
    };

    // Raycast click
    const raycaster = new THREE.Raycaster();
    const mouse = new THREE.Vector2();

    const onClick = (e: MouseEvent) => {
      const rect = dom.getBoundingClientRect();
      mouse.x = ((e.clientX - rect.left) / rect.width) * 2 - 1;
      mouse.y = -((e.clientY - rect.top) / rect.height) * 2 + 1;

      raycaster.setFromCamera(mouse, camera);
      const hitBoxes: THREE.Object3D[] = [];
      buildingMeshesRef.current.forEach((grp) => {
        const hb = grp.children.find((c) => c.userData.nodeId);
        if (hb) hitBoxes.push(hb);
      });

      const intersects = raycaster.intersectObjects(hitBoxes, false);
      if (intersects.length > 0) {
        const clickedNodeId = intersects[0].object.userData.nodeId;
        onSelectNode(clickedNodeId === selectedNodeId ? null : clickedNodeId);
      } else {
        onSelectNode(null);
      }
    };

    dom.addEventListener('mousedown', onMouseDown);
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
    dom.addEventListener('wheel', onWheel, { passive: false });
    dom.addEventListener('click', onClick);

    // Animation Loop
    let clock = new THREE.Clock();
    const animate = () => {
      animFrameIdRef.current = requestAnimationFrame(animate);
      const delta = clock.getDelta();

      // Animate packet particles along bezier curves
      packetParticlesRef.current.forEach((p) => {
        p.progress = (p.progress + delta * p.speed) % 1;
        const pt = p.curve.getPoint(p.progress);
        p.mesh.position.set(pt.x, pt.y, pt.z);
      });

      // Slowly rotate leader rings
      leaderRingsRef.current.forEach((r) => {
        r.rotation.z += delta * 0.8;
      });

      renderer.render(scene, camera);
    };
    animate();

    // Handle Resize responsively via ResizeObserver and window events
    const handleResize = () => {
      if (!mountRef.current) return;
      const w = mountRef.current.clientWidth;
      const h = mountRef.current.clientHeight;
      if (w === 0 || h === 0) return;
      camera.aspect = w / h;
      camera.updateProjectionMatrix();
      renderer.setSize(w, h);
    };
    window.addEventListener('resize', handleResize);

    const resizeObserver = new ResizeObserver(handleResize);
    resizeObserver.observe(mountRef.current);

    return () => {
      if (animFrameIdRef.current) cancelAnimationFrame(animFrameIdRef.current);
      dom.removeEventListener('mousedown', onMouseDown);
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
      dom.removeEventListener('wheel', onWheel);
      dom.removeEventListener('click', onClick);
      window.removeEventListener('resize', handleResize);
      resizeObserver.disconnect();

      // Deep GPU Resource Disposal to prevent WebGL memory leak
      scene.traverse((obj) => {
        if (obj instanceof THREE.Mesh || obj instanceof THREE.Line || obj instanceof THREE.Sprite) {
          if (obj.geometry) obj.geometry.dispose();
          if (Array.isArray(obj.material)) {
            obj.material.forEach((m) => m.dispose());
          } else if (obj.material) {
            obj.material.dispose();
          }
        }
      });

      if (renderer.domElement.parentNode) {
        renderer.domElement.parentNode.removeChild(renderer.domElement);
      }
      renderer.dispose();
    };
  }, [viewMode]);

  // Sync Nodes in 3D Scene
  useEffect(() => {
    const scene = sceneRef.current;
    if (!scene || viewMode !== '3d') return;

    leaderRingsRef.current = [];
    buildingMeshesRef.current.forEach((grp) => {
      scene.remove(grp);
      grp.traverse((child) => {
        if (child instanceof THREE.Mesh || child instanceof THREE.Sprite) {
          if (child.geometry) child.geometry.dispose();
          if (Array.isArray(child.material)) {
            child.material.forEach((m) => {
              if ('map' in m && m.map) m.map.dispose();
              m.dispose();
            });
          } else if (child.material) {
            if ('map' in child.material && (child.material as any).map) {
              (child.material as any).map.dispose();
            }
            child.material.dispose();
          }
        }
      });
    });
    buildingMeshesRef.current.clear();

    nodes.forEach((node) => {
      const isSelected = selectedNodeId === node.id;
      const building = createServerBuilding(node, isSelected);

      if (selectedNodeId) {
        if (isSelected) {
          building.scale.set(1.15, 1.15, 1.15);
        } else {
          building.scale.set(0.9, 0.9, 0.9);
        }
      } else {
        building.scale.set(1.0, 1.0, 1.0);
      }

      scene.add(building);
      buildingMeshesRef.current.set(node.id, building);
    });
  }, [nodes, selectedNodeId, viewMode]);

  // Sync Network Connections & Packet Photons in 3D Scene
  useEffect(() => {
    const scene = sceneRef.current;
    if (!scene || viewMode !== '3d') return;

    linkLinesRef.current.forEach(({ line }) => {
      scene.remove(line);
      if (line.geometry) line.geometry.dispose();
      if (line.material) {
        if (Array.isArray(line.material)) line.material.forEach((m) => m.dispose());
        else line.material.dispose();
      }
    });
    linkLinesRef.current.clear();

    packetParticlesRef.current.forEach((p) => {
      scene.remove(p.mesh);
      if (p.mesh.geometry) p.mesh.geometry.dispose();
      if (p.mesh.material) {
        if (Array.isArray(p.mesh.material)) p.mesh.material.forEach((m) => m.dispose());
        else p.mesh.material.dispose();
      }
    });
    packetParticlesRef.current = [];

    const nodeMap = new Map(nodes.map((n) => [n.id, n]));

    links.forEach((link) => {
      const srcNode = nodeMap.get(link.source);
      const dstNode = nodeMap.get(link.destination);
      if (!srcNode || !dstNode) return;

      const p1 = new THREE.Vector3(srcNode.position3D[0], 0.8, srcNode.position3D[2]);
      const p2 = new THREE.Vector3(dstNode.position3D[0], 0.8, dstNode.position3D[2]);

      // Subtle arc midpoint
      const mid = new THREE.Vector3()
        .addVectors(p1, p2)
        .multiplyScalar(0.5);
      mid.y += 0.8; // arch height

      const curve = new THREE.QuadraticBezierCurve3(p1, mid, p2);
      const points = curve.getPoints(32);
      const lineGeo = new THREE.BufferGeometry().setFromPoints(points);

      // Color selection
      let linkColor = 0x0891d5; // Default blue
      if (link.type === 'raft') {
        if (link.shardId === 0) linkColor = 0x0891d5;
        if (link.shardId === 1) linkColor = 0x7965e8;
        if (link.shardId === 2) linkColor = 0x10b981;
      }
      if (link.status === 'disconnected') {
        linkColor = 0xef4444;
      }

      const isHovered = hoveredLinkId === link.id;
      const isConnectedToSelected =
        selectedNodeId && (link.source === selectedNodeId || link.destination === selectedNodeId);

      const lineMat = new THREE.LineBasicMaterial({
        color: isHovered ? 0x0891d5 : linkColor,
        linewidth: isHovered || isConnectedToSelected ? 3 : 1,
        transparent: true,
        opacity:
          link.status === 'disconnected'
            ? 0.3
            : selectedNodeId
            ? isConnectedToSelected
              ? 1.0
              : 0.15
            : 0.7,
      });

      const line = new THREE.Line(lineGeo, lineMat);
      scene.add(line);
      linkLinesRef.current.set(link.id, { line, curve });

      // Add animated packet photon particle if active
      if (link.activeTraffic && link.status === 'healthy') {
        const particleGeo = new THREE.SphereGeometry(0.12, 12, 12);
        const particleMat = new THREE.MeshBasicMaterial({ color: linkColor });
        const particleMesh = new THREE.Mesh(particleGeo, particleMat);
        scene.add(particleMesh);

        packetParticlesRef.current.push({
          mesh: particleMesh,
          curve,
          progress: Math.random(),
          speed: 0.35 + Math.random() * 0.15,
        });
      }
    });
  }, [links, nodes, selectedNodeId, hoveredLinkId, viewMode]);

  return (
    <div className="relative w-full h-[520px] lg:h-[620px] bg-[#F4F7FC] rounded-2xl border border-[#DCE5F0] overflow-hidden shadow-xs">
      {viewMode === '3d' ? (
        <div ref={mountRef} className="w-full h-full cursor-grab active:cursor-grabbing" />
      ) : (
        /* 2D Flat Graph View */
        <div className="w-full h-full p-6 flex flex-col justify-between select-none relative bg-white">
          <svg className="w-full h-full">
            <defs>
              <pattern id="grid2d" width="40" height="40" patternUnits="userSpaceOnUse">
                <path d="M 40 0 L 0 0 0 40" fill="none" stroke="#E2E8F0" strokeWidth="0.8" />
              </pattern>
            </defs>
            <rect width="100%" height="100%" fill="url(#grid2d)" />

            {/* Links */}
            {links.map((link) => {
              const src = nodes.find((n) => n.id === link.source);
              const dst = nodes.find((n) => n.id === link.destination);
              if (!src || !dst) return null;

              const isHovered = hoveredLinkId === link.id;
              let strokeColor = '#0891D5';
              if (link.type === 'raft') {
                if (link.shardId === 0) strokeColor = '#0891D5';
                if (link.shardId === 1) strokeColor = '#7965E8';
                if (link.shardId === 2) strokeColor = '#10B981';
              }
              if (link.status === 'disconnected') strokeColor = '#EF4444';

              return (
                <g
                  key={link.id}
                  onMouseEnter={(e) => {
                    onHoverLink(link.id);
                    setHoveredLinkTooltip({ link, x: e.clientX, y: e.clientY });
                  }}
                  onMouseLeave={() => {
                    onHoverLink(null);
                    setHoveredLinkTooltip(null);
                  }}
                >
                  <line
                    x1={`${src.position2D.x}%`}
                    y1={`${src.position2D.y}%`}
                    x2={`${dst.position2D.x}%`}
                    y2={`${dst.position2D.y}%`}
                    stroke={strokeColor}
                    strokeWidth={isHovered ? 4 : 2}
                    strokeDasharray={link.status === 'disconnected' ? '6,6' : undefined}
                    strokeOpacity={link.status === 'disconnected' ? 0.35 : 0.85}
                    className="transition-all cursor-pointer"
                  />
                </g>
              );
            })}

            {/* Nodes */}
            {nodes.map((node) => {
              const isSelected = selectedNodeId === node.id;
              const isLeader = node.hostedShards.some((s) => s.role === 'Leader');

              return (
                <g
                  key={node.id}
                  onClick={() => onSelectNode(isSelected ? null : node.id)}
                  className="cursor-pointer group"
                >
                  <circle
                    cx={`${node.position2D.x}%`}
                    cy={`${node.position2D.y}%`}
                    r={isSelected ? 32 : 26}
                    fill={node.online ? '#FFFFFF' : '#FEE2E2'}
                    stroke={
                      isSelected
                        ? '#0891D5'
                        : isLeader
                        ? '#F59E0B'
                        : node.online
                        ? '#CBD5E1'
                        : '#EF4444'
                    }
                    strokeWidth={isSelected ? 4 : 2.5}
                    className="transition-all shadow-md"
                  />

                  {/* Leader amber ring */}
                  {isLeader && node.online && (
                    <circle
                      cx={`${node.position2D.x}%`}
                      cy={`${node.position2D.y}%`}
                      r={isSelected ? 38 : 32}
                      fill="none"
                      stroke="#F59E0B"
                      strokeWidth={1.5}
                      strokeDasharray="4,4"
                    />
                  )}

                  {/* Node Label */}
                  <text
                    x={`${node.position2D.x}%`}
                    y={`${node.position2D.y}%`}
                    dy="5"
                    textAnchor="middle"
                    className="font-mono font-bold text-xs select-none fill-[#17243A]"
                  >
                    {node.name.replace('Node ', '')}
                  </text>

                  {/* Subtext info */}
                  <text
                    x={`${node.position2D.x}%`}
                    y={`${node.position2D.y + 7}%`}
                    textAnchor="middle"
                    className="font-mono font-semibold text-[11px] select-none fill-[#64748B]"
                  >
                    {node.name}
                  </text>
                </g>
              );
            })}
          </svg>
        </div>
      )}

      {/* Top Floating Overlay Badge */}
      <div className="absolute top-4 left-4 pointer-events-none flex items-center gap-2">
        <span className="px-3 py-1 rounded-full bg-white/90 backdrop-blur-md border border-[#DCE5F0] text-xs font-mono font-semibold text-[#17243A] shadow-xs">
          {viewMode === '3d' ? '3D Physical Cluster' : '2D Schematic Mesh'} •{' '}
          <span className="text-[#0891D5] capitalize">{networkMode} View</span>
        </span>
      </div>

      {/* Helper Legend / Hint */}
      <div className="absolute bottom-4 left-4 pointer-events-none hidden sm:flex items-center gap-3 px-3 py-1.5 rounded-xl bg-white/90 backdrop-blur-md border border-[#DCE5F0] text-[11px] font-mono text-[#64748B] shadow-xs">
        <span className="flex items-center gap-1.5">
          <span className="w-2 h-2 rounded-full bg-[#10B981]" /> Online
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-2 h-2 rounded-full bg-[#EF4444]" /> Offline
        </span>
        <span className="flex items-center gap-1.5">
          <span className="w-2 h-2 rounded-full bg-[#F59E0B]" /> Shard Leader
        </span>
        <span className="border-l border-[#DCE5F0] pl-2 text-[10px]">
          Drag to rotate • Scroll to zoom • Click node to inspect
        </span>
      </div>
    </div>
  );
}
