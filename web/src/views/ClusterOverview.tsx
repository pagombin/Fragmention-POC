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
          <CardHeader><CardTitle>Topology</CardTitle></CardHeader>
          <CardBody>
            <div className="flex items-center gap-2 text-sm">
              <Badge tone="info">{data.topology.kind}</Badge>
              {data.topology.replica_set && <Badge>{data.topology.replica_set}</Badge>}
              {data.is_srv && <Badge tone="warn">mongodb+srv</Badge>}
            </div>
            <div className="mt-2 font-mono text-xs break-all text-slate-500">{data.redacted_uri}</div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader><CardTitle>Server</CardTitle></CardHeader>
          <CardBody>
            <dl className="grid grid-cols-2 gap-1 text-sm">
              <dt className="text-slate-500">Version</dt><dd className="font-mono">{data.server_info.Version}</dd>
              <dt className="text-slate-500">FCV</dt><dd className="font-mono">{data.server_info.FCV || '—'}</dd>
              <dt className="text-slate-500">Storage engine</dt><dd className="font-mono">{data.server_info.StorageEngine || '—'}</dd>
              <dt className="text-slate-500">Git</dt><dd className="font-mono text-xs">{data.server_info.GitVersion || '—'}</dd>
            </dl>
          </CardBody>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader><CardTitle>Members</CardTitle></CardHeader>
        <CardBody>
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-slate-500">
                <th className="py-1">Name</th><th>State</th><th>Health</th><th>Lag (s)</th><th>Self</th>
              </tr>
            </thead>
            <tbody>
              {data.topology.members.map((m) => (
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
