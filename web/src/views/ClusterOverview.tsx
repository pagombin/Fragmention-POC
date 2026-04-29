import { useTopology } from '../api/hooks';
import { Badge, Card, CardBody, CardHeader, CardTitle } from '../components/ui/primitives';

export function ClusterOverview() {
  const { data, isLoading, isError, error } = useTopology();

  if (isLoading) return <Page>Loading…</Page>;
  if (isError) {
    return (
      <Page>
        <Card>
          <CardBody>
            <div className="text-red-500">Failed to load topology: {String(error)}</div>
            <div className="mt-2 text-xs text-slate-500">
              Check that the server is running and the bearer token is set (top bar).
            </div>
          </CardBody>
        </Card>
      </Page>
    );
  }
  if (!data) return <Page>No data.</Page>;

  return (
    <Page>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card>
          <CardHeader><CardTitle title="Detected cluster shape and the connection URI in use">Topology</CardTitle></CardHeader>
          <CardBody>
            <div className="flex items-center gap-2 text-sm">
              <Badge tone="info" title="standalone = single-node mongod; replica_set = RS with members; sharded clusters are flagged but not driven">{data.topology.kind}</Badge>
              {data.topology.replica_set && <Badge title="Replica set name from `setName`">{data.topology.replica_set}</Badge>}
              {data.is_srv && <Badge tone="warn" title="DNS seedlist (mongodb+srv://); SRV resolution is performed by the driver. Connect timeout auto-bumps to 30s.">mongodb+srv</Badge>}
            </div>
            <div className="mt-2 font-mono text-xs break-all text-slate-500" title="Connection URI with passwords redacted">{data.redacted_uri}</div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader><CardTitle title="MongoDB server identity from `buildInfo`">Server</CardTitle></CardHeader>
          <CardBody>
            <dl className="grid grid-cols-2 gap-1 text-sm">
              <dt className="text-slate-500" title="MongoDB build version">Version</dt><dd className="font-mono">{data.server_info.Version}</dd>
              <dt className="text-slate-500" title="featureCompatibilityVersion - controls which feature flags the server enables">FCV</dt><dd className="font-mono">{data.server_info.FCV || '—'}</dd>
              <dt className="text-slate-500" title="Storage engine in use (always WiredTiger on supported versions)">Storage engine</dt><dd className="font-mono">{data.server_info.StorageEngine || '—'}</dd>
              <dt className="text-slate-500" title="MongoDB build git revision">Git</dt><dd className="font-mono text-xs">{data.server_info.GitVersion || '—'}</dd>
            </dl>
          </CardBody>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader><CardTitle title="Per-member status from rs.status() / hello (managed clusters fall back to hello.hosts)">Members</CardTitle></CardHeader>
        <CardBody>
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-slate-500">
                <th className="py-1" title="Host:port from rs.status">Name</th>
                <th title="Member state: PRIMARY, SECONDARY, ARBITER, STARTUP2 (initial sync), DOWN, etc.">State</th>
                <th title="1 = healthy, 0 = unreachable">Health</th>
                <th title="Replication lag in seconds vs. PRIMARY's optime">Lag (s)</th>
                <th title="True if the dashboard's connection lands on this member">Self</th>
              </tr>
            </thead>
            <tbody>
              {(data.topology.members ?? []).map((m) => (
                <tr key={m.name} className="border-t border-slate-200/60 dark:border-slate-800">
                  <td className="py-1 font-mono text-xs">{m.name}</td>
                  <td><Badge tone={m.state === 'PRIMARY' ? 'success' : m.state === 'SECONDARY' ? 'info' : 'warn'}>{m.state}</Badge></td>
                  <td className="font-mono text-xs">{m.health}</td>
                  <td className="font-mono text-xs">{m.lag_seconds?.toFixed(2) ?? '—'}</td>
                  <td>{m.self ? '•' : ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.topology.sharded && (
            <div className="mt-3 rounded border border-yellow-500 bg-yellow-50 p-2 text-xs text-yellow-900 dark:bg-yellow-900/30 dark:text-yellow-100">
              Sharded cluster detected. mfpoc does not support sharded clusters (spec § 18).
            </div>
          )}
        </CardBody>
      </Card>
    </Page>
  );
}

export function Page({ children }: { children: React.ReactNode }) {
  return <div className="flex-1 overflow-auto p-4">{children}</div>;
}
