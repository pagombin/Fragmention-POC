import { useMemo, useState } from 'react';
import { Page } from './ClusterOverview';
import { Badge, Card, CardBody, CardHeader, CardTitle, Select, Table, Td, Th } from '../components/ui/primitives';
import { useRunMetrics, useRuns } from '../api/hooks';
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid, Legend } from 'recharts';

export function CompareRuns() {
  const runs = useRuns();
  const [a, setA] = useState<string>('');
  const [b, setB] = useState<string>('');

  const sampA = useRunMetrics(a || null, { metric: 'fragmentation_ratio', scope: 'cluster' });
  const sampB = useRunMetrics(b || null, { metric: 'fragmentation_ratio', scope: 'cluster' });

  const chartData = useMemo(() => {
    const base: Record<number, { t: number; A?: number; B?: number }> = {};
    sampA.data?.forEach((s) => {
      const t = new Date(s.timestamp).getTime();
      base[t] = { ...(base[t] ?? { t }), A: s.value };
    });
    sampB.data?.forEach((s) => {
      const t = new Date(s.timestamp).getTime();
      base[t] = { ...(base[t] ?? { t }), B: s.value };
    });
    return Object.values(base).sort((x, y) => x.t - y.t);
  }, [sampA.data, sampB.data]);

  return (
    <Page>
      <h2 className="text-lg font-semibold mb-3">Compare runs</h2>
      <Card>
        <CardHeader><CardTitle>Runs</CardTitle></CardHeader>
        <CardBody>
          {(runs.data?.length ?? 0) < 2 && (
            <div className="mb-3 rounded border border-yellow-500 bg-yellow-50 p-2 text-xs text-yellow-900 dark:bg-yellow-900/30 dark:text-yellow-100">
              You need at least two runs to compare. Create one from the New Run Wizard.
            </div>
          )}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <div className="text-xs uppercase text-slate-500 mb-1" title="The 'reference' run for the comparison">Run A</div>
              <Select value={a} onChange={(e) => setA(e.target.value)} disabled={(runs.data?.length ?? 0) === 0}>
                <option value="">—</option>
                {runs.data?.filter((r) => r.id !== b).map((r) => (
                  <option key={r.id} value={r.id}>{r.name}</option>
                ))}
              </Select>
            </div>
            <div>
              <div className="text-xs uppercase text-slate-500 mb-1" title="The run to compare against A">Run B</div>
              <Select
                value={b}
                onChange={(e) => setB(e.target.value)}
                disabled={!a || (runs.data?.filter((r) => r.id !== a).length ?? 0) === 0}
                title={!a ? 'Select Run A first' : undefined}
              >
                <option value="">—</option>
                {runs.data?.filter((r) => r.id !== a).map((r) => (
                  <option key={r.id} value={r.id}>{r.name}</option>
                ))}
              </Select>
            </div>
          </div>
        </CardBody>
      </Card>

      {(a || b) && (
        <Card className="mt-4">
          <CardHeader><CardTitle>Fragmentation over time</CardTitle></CardHeader>
          <CardBody style={{ height: 320 }}>
            {chartData.length === 0 ? (
              <div className="text-sm text-slate-500">No samples for the selected runs.</div>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={chartData}>
                  <CartesianGrid strokeOpacity={0.1} />
                  <XAxis dataKey="t" tickFormatter={(t) => new Date(t).toLocaleTimeString()} fontSize={10} />
                  <YAxis domain={[0, 1]} tickFormatter={(v) => `${Math.round(v * 100)}%`} fontSize={10} />
                  <Tooltip labelFormatter={(l) => new Date(l as number).toLocaleString()} />
                  <Legend />
                  <Line type="monotone" dataKey="A" stroke="#3b6dff" dot={false} isAnimationActive={false} />
                  <Line type="monotone" dataKey="B" stroke="#dc2626" dot={false} isAnimationActive={false} />
                </LineChart>
              </ResponsiveContainer>
            )}
          </CardBody>
        </Card>
      )}

      <Card className="mt-4">
        <CardHeader><CardTitle>Summary</CardTitle></CardHeader>
        <CardBody>
          <Table>
            <thead><tr><Th>Run</Th><Th>Status</Th><Th>Created</Th><Th>Completed</Th></tr></thead>
            <tbody>
              {[a, b].filter(Boolean).map((rid) => {
                const r = runs.data?.find((x) => x.id === rid);
                if (!r) return null;
                return (
                  <tr key={rid}>
                    <Td className="font-mono">{r.name}</Td>
                    <Td><Badge>{r.status}</Badge></Td>
                    <Td className="text-xs">{new Date(r.created_at).toLocaleString()}</Td>
                    <Td className="text-xs">{r.completed_at ? new Date(r.completed_at).toLocaleString() : '—'}</Td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </CardBody>
      </Card>
    </Page>
  );
}
