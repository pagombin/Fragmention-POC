import { useParams } from 'react-router-dom';
import { Page } from './ClusterOverview';
import { Badge, Card, CardBody, CardHeader, CardTitle } from '../components/ui/primitives';
import { useEvents, useRun, useRunMetrics } from '../api/hooks';
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from 'recharts';
import { useMemo } from 'react';

export function RunDetail() {
  const { id } = useParams<{ id: string }>();
  const run = useRun(id ?? null);
  const samples = useRunMetrics(id ?? null, { metric: 'fragmentation_ratio', scope: 'cluster' });
  const events = useEvents(undefined, id ?? undefined);

  const chartData = useMemo(() => {
    if (!samples.data) return [];
    return samples.data.map((s) => ({ t: new Date(s.timestamp).getTime(), v: s.value }));
  }, [samples.data]);

  if (run.isLoading) return <Page>Loading…</Page>;
  if (!run.data) return <Page><Card><CardBody>Run not found.</CardBody></Card></Page>;
  const r = run.data;

  return (
    <Page>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold">{r.name}</h2>
        <div className="text-xs text-slate-500">
          <Badge>{r.status}</Badge> · created {new Date(r.created_at).toLocaleString()}
        </div>
      </div>

      <Card>
        <CardHeader><CardTitle>Cluster fragmentation</CardTitle></CardHeader>
        <CardBody style={{ height: 260 }}>
          {chartData.length === 0 ? (
            <div className="text-sm text-slate-500">No samples associated with this run yet.</div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData}>
                <CartesianGrid strokeOpacity={0.1} />
                <XAxis dataKey="t" tickFormatter={(t) => new Date(t).toLocaleTimeString()} fontSize={10} />
                <YAxis domain={[0, 1]} tickFormatter={(v) => `${Math.round(v * 100)}%`} fontSize={10} />
                <Tooltip
                  labelFormatter={(l) => new Date(l as number).toLocaleString()}
                  formatter={(v: number) => [`${(v * 100).toFixed(2)}%`, 'fragmentation']}
                />
                <Line type="monotone" dataKey="v" stroke="#3b6dff" dot={false} isAnimationActive={false} />
              </LineChart>
            </ResponsiveContainer>
          )}
        </CardBody>
      </Card>

      <Card className="mt-4">
        <CardHeader><CardTitle>Events</CardTitle></CardHeader>
        <CardBody>
          {events.data?.length === 0 && <div className="text-slate-500 text-sm">No events.</div>}
          <ul className="space-y-0.5 text-xs font-mono">
            {events.data?.map((e) => (
              <li key={e.id} className="flex gap-2">
                <span className="text-slate-500">{new Date(e.timestamp).toLocaleTimeString()}</span>
                <Badge tone={e.level === 'error' ? 'error' : e.level === 'warn' ? 'warn' : 'info'}>{e.level}</Badge>
                <span>{e.category}</span>
                <span>{e.message}</span>
              </li>
            ))}
          </ul>
        </CardBody>
      </Card>
    </Page>
  );
}
