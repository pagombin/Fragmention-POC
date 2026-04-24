import { useState } from 'react';
import { Page } from './ClusterOverview';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Input, Label, Select } from '../components/ui/primitives';
import { useAuth } from '../stores/auth';
import { useUI } from '../stores/ui';
import { api } from '../api/client';
import { useTopology } from '../api/hooks';

export function Settings() {
  const { token, setToken } = useAuth();
  const { theme, setTheme } = useUI();
  const [logLevel, setLogLevel] = useState<'debug' | 'info' | 'warn' | 'error'>('info');
  const [status, setStatus] = useState<string>('');
  const topology = useTopology();

  const applyLogLevel = async () => {
    try {
      await api<{ previous: string; current: string }>('/api/v1/admin/log-level', {
        method: 'POST',
        body: JSON.stringify({ level: logLevel }),
      });
      setStatus(`log level set to ${logLevel}`);
    } catch (err) {
      setStatus(`failed: ${String(err)}`);
    }
  };

  return (
    <Page>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card>
          <CardHeader><CardTitle>Auth token</CardTitle></CardHeader>
          <CardBody>
            <Label>Bearer token</Label>
            <Input type="password" value={token ?? ''} onChange={(e) => setToken(e.target.value || null)} />
            <div className="mt-2 text-xs text-slate-500">Stored in localStorage. Log out with the Clear button.</div>
            <div className="mt-2"><Button variant="ghost" size="sm" onClick={() => setToken(null)}>Clear token</Button></div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader><CardTitle>Theme</CardTitle></CardHeader>
          <CardBody>
            <Select value={theme} onChange={(e) => setTheme(e.target.value as 'dark' | 'light')}>
              <option value="dark">dark</option>
              <option value="light">light</option>
            </Select>
          </CardBody>
        </Card>

        <Card>
          <CardHeader><CardTitle>Log level (runtime)</CardTitle></CardHeader>
          <CardBody>
            <div className="flex items-center gap-2">
              <Select value={logLevel} onChange={(e) => setLogLevel(e.target.value as typeof logLevel)}>
                <option>debug</option><option>info</option><option>warn</option><option>error</option>
              </Select>
              <Button variant="primary" size="sm" onClick={applyLogLevel}>Apply</Button>
              {status && <span className="text-xs text-slate-500">{status}</span>}
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader><CardTitle>Cluster info</CardTitle></CardHeader>
          <CardBody>
            <dl className="grid grid-cols-2 gap-1 text-sm">
              <dt className="text-slate-500">Topology</dt><dd>{topology.data?.topology.kind ?? '—'}</dd>
              <dt className="text-slate-500">Mongo</dt><dd className="font-mono">{topology.data?.server_info.Version ?? '—'}</dd>
              <dt className="text-slate-500">SRV</dt><dd>{topology.data?.is_srv ? <Badge tone="warn">yes</Badge> : 'no'}</dd>
              <dt className="text-slate-500">URI (redacted)</dt><dd className="font-mono text-xs break-all">{topology.data?.redacted_uri ?? '—'}</dd>
            </dl>
          </CardBody>
        </Card>
      </div>
    </Page>
  );
}
