/**
 * Standardized ORBIT API Error Architecture
 */

export class OrbitApiError extends Error {
  public readonly status: number;
  public readonly code: string;
  public readonly requestId?: string;
  public readonly endpoint?: string;
  public readonly rawBody?: unknown;

  constructor(options: {
    message: string;
    status: number;
    code?: string;
    requestId?: string;
    endpoint?: string;
    rawBody?: unknown;
  }) {
    super(options.message);
    this.name = 'OrbitApiError';
    this.status = options.status;
    this.code = options.code || (options.status >= 500 ? 'INTERNAL_ERROR' : 'API_ERROR');
    this.requestId = options.requestId;
    this.endpoint = options.endpoint;
    this.rawBody = options.rawBody;

    Object.setPrototypeOf(this, new.target.prototype);
  }

  public toJSON() {
    return {
      name: this.name,
      message: this.message,
      status: this.status,
      code: this.code,
      requestId: this.requestId,
      endpoint: this.endpoint,
    };
  }
}

export class NetworkError extends OrbitApiError {
  constructor(message: string = 'Network connection failed or node unreachable', endpoint?: string) {
    super({
      message,
      status: 0,
      code: 'NETWORK_ERROR',
      endpoint,
    });
    this.name = 'NetworkError';
  }
}

export class TimeoutError extends OrbitApiError {
  constructor(timeoutMs: number, endpoint?: string) {
    super({
      message: `Request timed out after ${timeoutMs}ms`,
      status: 408,
      code: 'TIMEOUT',
      endpoint,
    });
    this.name = 'TimeoutError';
  }
}

export class ClusterOfflineError extends OrbitApiError {
  constructor(message: string = 'Cluster offline: none of the 5 nodes responded') {
    super({
      message,
      status: 503,
      code: 'CLUSTER_OFFLINE',
    });
    this.name = 'ClusterOfflineError';
  }
}

export class NotFoundError extends OrbitApiError {
  constructor(key: string, requestId?: string) {
    super({
      message: `Key "${key}" not found in database`,
      status: 404,
      code: 'KEY_NOT_FOUND',
      requestId,
    });
    this.name = 'NotFoundError';
  }
}

export class QuorumLostError extends OrbitApiError {
  constructor(shardId: number | string, requestId?: string) {
    super({
      message: `Raft quorum lost for Shard ${shardId}: majority of replicas unavailable`,
      status: 503,
      code: 'QUORUM_LOST',
      requestId,
    });
    this.name = 'QuorumLostError';
  }
}

/**
 * Normalizes any caught exception into a structured OrbitApiError.
 */
export function normalizeError(err: unknown, endpoint?: string): OrbitApiError {
  if (err instanceof OrbitApiError) {
    return err;
  }

  if (err instanceof DOMException && err.name === 'AbortError') {
    return new OrbitApiError({
      message: 'Request was cancelled by client',
      status: 499,
      code: 'REQUEST_CANCELLED',
      endpoint,
    });
  }

  if (err instanceof TypeError && err.message.toLowerCase().includes('fetch')) {
    return new NetworkError(err.message, endpoint);
  }

  if (err instanceof Error) {
    return new OrbitApiError({
      message: err.message,
      status: 500,
      code: 'UNKNOWN_ERROR',
      endpoint,
    });
  }

  return new OrbitApiError({
    message: String(err) || 'An unexpected error occurred',
    status: 500,
    code: 'UNKNOWN_ERROR',
    endpoint,
  });
}
