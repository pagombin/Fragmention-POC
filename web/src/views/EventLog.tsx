import { useState } from 'react';
import { Page } from './ClusterOverview';
import { Badge, Card, CardBody, Input, Label, Select } from '../components/ui/primitives';
import { useEvents } from '../api/hooks';

export function EventLog() {
  const [category, setCategory] = useState('');
  const [runID, setRunID] = useState('');
  const events = useEvents(category || undefined, runID || undefined);

  return (
    <Page>
      <div className="mb-3 flex items-end gap-2">
        <div>
          <Label>Category</Label>
          <Select value={category} onChange={(e) => setCategory(e.target.value)}>
            <option value="">all</option>
            {['loader_started','loader_paused','loader_resumed','loader_stopped','loader_completed','loader_failed',
              'deleter_started','deleter_paused','deleter_resumed','deleter_stopped','deleter_completed','deleter_failed',
              'compact_started','compact_stopped','compact_completed','compact_failed',
              'workload_started','workload_stopped','workload_completed',
              'snapshot','stepdown_initiated','auth'].map((c) => <option key={c} value={c}>{c}</option>)}
          </Select>
        </div>
        <div>
          <Label>Run ID filter</Label>
          <Input value={runID} onChange={(e) => setRunID(e.target.value)} placeholder="run uuid" className="w-72" />
        </div>
      </div>

      <Card>
        <CardBody>
          {events.isLoading && <div className="text-slate-500 text-sm">Loading…</div>}
          {events.data?.length === 0 && <div className="text-slate-500 text-sm">No events match.</div>}
          <table className="w-full text-xs font-mono">
            <thead>
              <tr className="text-left text-slate-500 uppercase tracking-wide">
                <th className="py-1">Time</th>
                <th>Level</th>
                <th>Category</th>
                <th>Run</th>
                <th>Op</th>
                <th>Message</th>
              </tr>
            </thead>
            <tbody>
              {events.data?.map((e) => (
                <tr key={e.id} className="border-t border-slate-100 dark:border-slate-900">
                  <td className="py-0.5">{new Date(e.timestamp).toLocaleString()}</td>
                  <td><Badge tone={e.level === 'error' ? 'error' : e.level === 'warn' ? 'warn' : e.level === 'audit' ? 'info' : 'neutral'}>{e.level}</Badge></td>
                  <td>{e.category}</td>
                  <td>{e.run_id ? e.run_id.slice(0, 8) : '—'}</td>
                  <td>{e.operation_id ? e.operation_id.slice(0, 8) : '—'}</td>
                  <td>{e.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardBody>
      </Card>
    </Page>
  );
}
