import { useEffect, useState } from 'react';
import { Button, Card, CardBody, Input, Label } from './ui/primitives';
import { api, ApiError } from '../api/client';
import { useAuth } from '../stores/auth';

// AuthGate detects whether the API is auth-required (probing /api/v1/version)
// and, when it is and the operator has no valid token, blocks the app behind
// a friendly login screen. The probe uses /api/v1/version because it is the
// cheapest auth-required endpoint - if it returns 401, we know auth is on
// and the current token (or absence of one) is rejected.
export function AuthGate({ children }: { children: React.ReactNode }) {
  const { token, setToken } = useAuth();
  const [state, setState] = useState<'checking' | 'ok' | 'needed'>('checking');
  const [draft, setDraft] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Probe on mount and whenever the token changes.
  useEffect(() => {
    let cancelled = false;
    setState('checking');
    setError(null);
    (async () => {
      try {
        await api('/api/v1/version');
        if (!cancelled) setState('ok');
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 401) {
          setState('needed');
        } else {
          // Network error / server down / something else: don't gate the
          // UI on it, let the app render and show its own error states.
          setState('ok');
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!draft.trim()) return;
    setSubmitting(true);
    setError(null);
    setToken(draft.trim());
    // The store update will retrigger the effect via the [token] dep.
    setSubmitting(false);
  };

  if (state === 'checking') {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-50 dark:bg-slate-950">
        <div className="text-sm text-slate-500">Checking authentication…</div>
      </div>
    );
  }

  if (state === 'needed') {
    return (
      <div className="flex h-screen items-center justify-center bg-slate-50 p-4 dark:bg-slate-950">
        <Card className="w-full max-w-md">
          <CardBody className="p-6">
            <div className="mb-4">
              <h1 className="text-xl font-semibold">Authentication required</h1>
              <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
                The mfpoc server has bearer-token auth enabled. Paste the token below.
              </p>
            </div>

            <form onSubmit={submit} className="space-y-3">
              <div>
                <Label>Bearer token</Label>
                <Input
                  type="password"
                  autoFocus
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  placeholder="paste token from /etc/mfpoc/mfpoc.env"
                  className="w-full"
                />
                {error && <div className="mt-1 text-xs text-red-500">{error}</div>}
              </div>
              <Button type="submit" variant="primary" className="w-full" disabled={submitting || !draft.trim()}>
                {submitting ? 'Saving…' : 'Sign in'}
              </Button>
            </form>

            <details className="mt-4 text-xs text-slate-500 dark:text-slate-400">
              <summary className="cursor-pointer">Where to find the token</summary>
              <div className="mt-2 space-y-1">
                <p>On the droplet:</p>
                <pre className="rounded bg-slate-100 p-2 font-mono text-xs dark:bg-slate-900">
{`ssh root@<droplet-ip> \\
  'sudo grep MFPOC_AUTH_BEARER_TOKEN /etc/mfpoc/mfpoc.env'`}
                </pre>
                <p>Or from your workstation: <code className="font-mono">make show-droplet-token DROPLET_IP=&lt;ip&gt; DROPLET_USER=root</code></p>
                <p>The token is generated once on first install and persists across redeploys. To rotate it, edit <code className="font-mono">/etc/mfpoc/mfpoc.env</code> and <code className="font-mono">systemctl restart mfpoc</code>.</p>
              </div>
            </details>
          </CardBody>
        </Card>
      </div>
    );
  }

  return <>{children}</>;
}
