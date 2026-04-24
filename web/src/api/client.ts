import { useAuth } from '../stores/auth';

export interface Envelope<T> {
  data: T | null;
  error: {
    code: string;
    message: string;
    details?: Record<string, unknown>;
  } | null;
}

export class ApiError extends Error {
  code: string;
  status: number;
  details?: Record<string, unknown>;
  constructor(status: number, code: string, message: string, details?: Record<string, unknown>) {
    super(message);
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

// api is the low-level fetch wrapper. It injects the bearer token from the
// auth store (when set), parses the uniform {data,error} envelope, and
// throws ApiError on non-2xx. Handlers retain .details so the UI can render
// rich conflict messages (e.g., scope-overlap).
export async function api<T = unknown>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const { token } = useAuth.getState();
  const headers = new Headers(init.headers);
  if (!headers.has('Content-Type') && init.body) {
    headers.set('Content-Type', 'application/json');
  }
  if (token) headers.set('Authorization', `Bearer ${token}`);

  const res = await fetch(path, { ...init, headers });
  const text = await res.text();
  let env: Envelope<T> | null = null;
  if (text) {
    try {
      env = JSON.parse(text) as Envelope<T>;
    } catch {
      // Non-JSON response body (e.g., plain error text from Mux).
    }
  }
  if (!res.ok) {
    const code = env?.error?.code ?? 'http_error';
    const message = env?.error?.message ?? (text || res.statusText);
    throw new ApiError(res.status, code, message, env?.error?.details);
  }
  return (env?.data ?? (undefined as unknown as T));
}

// wsURL returns the fully-qualified ws:// or wss:// URL for a server-side
// stream path, so the SPA works over both http and https droplet deploys.
export function wsURL(path: string): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${window.location.host}${path}`;
}
