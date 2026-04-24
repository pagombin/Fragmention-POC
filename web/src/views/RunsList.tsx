import { Link } from 'react-router-dom';
import { Page } from './ClusterOverview';
import { Badge, Button, Card, CardBody, Table, Td, Th } from '../components/ui/primitives';
import { useRuns } from '../api/hooks';
import { useNavigate } from 'react-router-dom';

export function RunsList() {
  const runs = useRuns();
  const nav = useNavigate();
  return (
    <Page>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-semibold">Runs</h2>
        <Button variant="primary" onClick={() => nav('/wizard')}>New run</Button>
      </div>
      <Card>
        <CardBody>
          {runs.isLoading && <div className="text-slate-500 text-sm">Loading…</div>}
          {runs.data?.length === 0 && <div className="text-slate-500 text-sm">No runs yet. Use the wizard to create one.</div>}
          {runs.data && runs.data.length > 0 && (
            <Table>
              <thead>
                <tr>
                  <Th>Name</Th><Th>Status</Th><Th>Created</Th><Th>Started</Th><Th>Completed</Th><Th></Th>
                </tr>
              </thead>
              <tbody>
                {runs.data.map((r) => (
                  <tr key={r.id}>
                    <Td className="font-mono">{r.name}</Td>
                    <Td><Badge tone={statusTone(r.status)}>{r.status}</Badge></Td>
                    <Td className="text-xs">{new Date(r.created_at).toLocaleString()}</Td>
                    <Td className="text-xs">{r.started_at ? new Date(r.started_at).toLocaleString() : '—'}</Td>
                    <Td className="text-xs">{r.completed_at ? new Date(r.completed_at).toLocaleString() : '—'}</Td>
                    <Td className="text-right"><Link className="text-brand-500 underline text-sm" to={`/runs/${r.id}`}>open</Link></Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </CardBody>
      </Card>
    </Page>
  );
}

function statusTone(s: string): 'success' | 'warn' | 'error' | 'info' | 'neutral' {
  if (s === 'completed') return 'success';
  if (s === 'running') return 'info';
  if (s === 'failed') return 'error';
  if (s === 'cancelled') return 'warn';
  return 'neutral';
}
