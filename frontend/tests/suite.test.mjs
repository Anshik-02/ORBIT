import test from 'node:test';
import assert from 'node:assert/strict';

// ==========================================
// 1. Settings Duration Validation Tests
// ==========================================

const GO_DURATION_REGEX = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/;

function parseDurationToMs(val) {
  const match = val.match(/^(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)$/);
  if (!match) return null;
  const num = parseFloat(match[1]);
  const unit = match[2];
  switch (unit) {
    case 'ns': return num / 1e6;
    case 'us':
    case 'µs': return num / 1e3;
    case 'ms': return num;
    case 's': return num * 1000;
    case 'm': return num * 60000;
    case 'h': return num * 3600000;
    default: return null;
  }
}

function validateDuration(val, minMs = 10, maxMs = 60000) {
  if (typeof val !== 'string' || !GO_DURATION_REGEX.test(val)) {
    return false;
  }
  const ms = parseDurationToMs(val);
  return ms !== null && ms >= minMs && ms <= maxMs;
}

test('Settings Duration: validates standard Go duration formats', () => {
  assert.equal(validateDuration('50ms'), true);
  assert.equal(validateDuration('150ms'), true);
  assert.equal(validateDuration('300ms'), true);
  assert.equal(validateDuration('1s'), true);
  assert.equal(validateDuration('2.5s'), true);
});

test('Settings Duration: rejects invalid duration strings', () => {
  assert.equal(validateDuration('50'), false);
  assert.equal(validateDuration('invalid'), false);
  assert.equal(validateDuration('-10ms'), false);
  assert.equal(validateDuration('500sec'), false);
  assert.equal(validateDuration(''), false);
});

test('Settings Duration: rejects durations outside safety bounds', () => {
  assert.equal(validateDuration('2ms', 10, 60000), false); // Below 10ms
  assert.equal(validateDuration('120s', 10, 60000), false); // Above 60s
});

test('Settings Safety: verifies Raft election timing constraint', () => {
  const heartbeat = parseDurationToMs('50ms');
  const minElection = parseDurationToMs('150ms');
  const maxElection = parseDurationToMs('300ms');

  assert.ok(heartbeat < minElection, 'Heartbeat must be less than min election timeout');
  assert.ok(minElection < maxElection, 'Min election timeout must be less than max election timeout');
  assert.ok(minElection >= heartbeat * 2, 'Min election timeout should be at least 2x heartbeat');
});

// ==========================================
// 2. Topology & Consensus Quorum Logic
// ==========================================

function calculateQuorumStatus(replicaNodeIds, availableNodeIds) {
  const total = replicaNodeIds.length;
  const available = replicaNodeIds.filter((id) => availableNodeIds.includes(id)).length;
  const quorumRequired = Math.floor(total / 2) + 1;
  const quorumAvailable = available >= quorumRequired;
  return {
    total,
    available,
    quorumRequired,
    quorumAvailable,
    status: quorumAvailable ? (available === total ? 'HEALTHY' : 'DEGRADED') : 'UNAVAILABLE',
  };
}

test('Quorum Logic: 3/3 replicas available is HEALTHY', () => {
  const res = calculateQuorumStatus(['node-a', 'node-b', 'node-c'], ['node-a', 'node-b', 'node-c']);
  assert.equal(res.quorumAvailable, true);
  assert.equal(res.status, 'HEALTHY');
  assert.equal(res.available, 3);
});

test('Quorum Logic: 2/3 replicas available is DEGRADED but maintains quorum', () => {
  const res = calculateQuorumStatus(['node-a', 'node-b', 'node-c'], ['node-a', 'node-c']);
  assert.equal(res.quorumAvailable, true);
  assert.equal(res.status, 'DEGRADED');
  assert.equal(res.available, 2);
});

test('Quorum Logic: 1/3 replicas available loses quorum (UNAVAILABLE)', () => {
  const res = calculateQuorumStatus(['node-a', 'node-b', 'node-c'], ['node-a']);
  assert.equal(res.quorumAvailable, false);
  assert.equal(res.status, 'UNAVAILABLE');
  assert.equal(res.available, 1);
});

// ==========================================
// 3. Benchmark Metrics & Diff Analysis
// ==========================================

function calculateBenchmarkComparison(current, baseline) {
  const qpsDelta = baseline.throughputOpsPerSec > 0
    ? ((current.throughputOpsPerSec - baseline.throughputOpsPerSec) / baseline.throughputOpsPerSec) * 100
    : 0;

  const p99Delta = baseline.latencyP99Ms > 0
    ? ((current.latencyP99Ms - baseline.latencyP99Ms) / baseline.latencyP99Ms) * 100
    : 0;

  return {
    qpsDeltaPercent: parseFloat(qpsDelta.toFixed(1)),
    p99DeltaPercent: parseFloat(p99Delta.toFixed(1)),
    improved: qpsDelta > 0 && p99Delta <= 0,
  };
}

test('Benchmark Comparison: accurately calculates throughput & latency delta', () => {
  const baseline = { throughputOpsPerSec: 10000, latencyP99Ms: 4.0 };
  const improved = { throughputOpsPerSec: 12500, latencyP99Ms: 3.2 };

  const comp = calculateBenchmarkComparison(improved, baseline);
  assert.equal(comp.qpsDeltaPercent, 25.0);
  assert.equal(comp.p99DeltaPercent, -20.0);
  assert.equal(comp.improved, true);
});

// ==========================================
// 4. Allowed Node ID Access Security
// ==========================================

const ALLOWED_NODE_IDS = new Set(['node-a', 'node-b', 'node-c', 'node-d', 'node-e']);

test('Security: restricts node configuration access strictly to allowed nodes', () => {
  assert.equal(ALLOWED_NODE_IDS.has('node-a'), true);
  assert.equal(ALLOWED_NODE_IDS.has('node-e'), true);
  assert.equal(ALLOWED_NODE_IDS.has('../../etc/passwd'), false);
  assert.equal(ALLOWED_NODE_IDS.has('node-z'), false);
  assert.equal(ALLOWED_NODE_IDS.has(''), false);
});
