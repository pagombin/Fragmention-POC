import { useState } from 'react';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Input, Label, formatNumber } from '../../components/ui/primitives';
import { TargetSelector } from '../../components/TargetSelector';
import { useActiveWorkloads, useWorkloadLifecycle, useWorkloadStart } from '../../api/hooks';
import { useSelection } from '../../stores/selection';
import { useUI } from '../../stores/ui';
import { ApiError } from '../../api/client';
import type { WorkloadStatus } from '../../api/types';

export function WorkloadTab() {
  const sel = useSelection();
  const pushToast = useUI((s) => s.pushToast);
  const start = useWorkloadStart();
  const active = useActiveWorkloads();

  const [opsPerSec, setOpsPerSec] = useState(100);
  const [workers, setWorkers] = useState(8);
  const [read, setRead] = useState(0.7);
  const [write, setWrite] = useState(0.2);
  const [agg, setAgg] = useState(0.1);

  const canStart = sel.selected.size > 0 && opsPerSec > 0 && workers > 0;

  const onStart = async () => {
    try {
      const res = await start.mutateAsync({
        spec: {
          targets: sel.entries(),
          read_weight: read,
          write_weight: write,
          aggregate_weight: agg,
        },
        params: { target_ops_per_sec: opsPerSec, workers },
      });
      pushToast({ kind: 'success', title: 'Workload started', body: `operation ${res.operation_id.slice(0, 8)}` });
    } catch (err) {
      const msg = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
      pushToast({ kind: 'error', title: 'Start failed', body: msg });
    }
  };

  return (
    <div className="grid grid-cols-1 xl:grid-cols-[2fr_3fr] gap-4">
      <TargetSelector />

      <div className="space-y-4">
        <Card>
          <CardHeader><CardTitle>Workload mix</CardTitle></CardHeader>
          <CardBody>
            <div className="grid grid-cols-2 gap-3">
              <WeightSlider label="Read weight" value={read} onChange={setRead} />
              <WeightSlider label="Write weight" value={write} onChange={setWrite} />
              <WeightSlider label="Aggregate weight" value={agg} onChange={setAgg} />
              <div>
                <Label>Target ops/sec</Label>
                <Input type="number" min={1} value={opsPerSec} onChange={(e) => setOpsPerSec(Math.max(1, Number(e.target.value)))} />
              </div>
              <div>
                <Label>Workers</Label>
                <Input type="number" min={1} value={workers} onChange={(e) => setWorkers(Math.max(1, Number(e.target.value)))} />
              </div>
            </div>

            <div className="mt-4 flex items-center gap-2">
              <Button variant="primary" onClick={onStart} disabled={!canStart || start.isPending}>
                {start.isPending ? 'Starting…' : 'Start workload'}
              </Button>
              {!canStart && <span className="text-xs text-slate-500">Pick at least one collection first.</span>}
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader><CardTitle>Active workloads</CardTitle></CardHeader>
          <CardBody>
            {active.isLoading && <div className="text-slate-500 text-sm">Loading…</div>}
            {active.data?.length === 0 && <div className="text-slate-500 text-sm">No active workloads.</div>}
            <div className="space-y-3">
              {active.data?.map((w) => <WorkloadCard key={w.operation_id} w={w} />)}
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
}

function WeightSlider({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  return (
    <div>
      <Label>{label}</Label>
      <div className="flex items-center gap-2">
        <input type="range" min={0} max={1} step={0.05} value={value} onChange={(e) => onChange(Number(e.target.value))} className="flex-1" />
        <Input type="number" step={0.05} min={0} max={1} value={value} onChange={(e) => onChange(Math.min(1, Math.max(0, Number(e.target.value))))} className="w-24" />
      </div>
    </div>
  );
}

function WorkloadCard({ w }: { w: WorkloadStatus }) {
  const life = useWorkloadLifecycle();
  const [rate, setRate] = useState(w.params.target_ops_per_sec);
  const [workers, setWorkers] = useState(w.params.workers);
  const pushToast = useUI((s) => s.pushToast);

  const doRate = async () => {
    try {
      await life.setRate.mutateAsync({ id: w.operation_id, params: { target_ops_per_sec: rate, workers } });
      pushToast({ kind: 'success', title: 'Rate updated' });
    } catch (err) {
      pushToast({ kind: 'error', title: 'Adjust failed', body: String(err) });
    }
  };

  return (
    <div className="rounded border border-slate-200 dark:border-slate-800 p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs">{w.operation_id.slice(0, 8)}</span>
          <Badge tone={w.state === 'running' ? 'success' : 'neutral'}>{w.state}</Badge>
        </div>
        {w.state === 'running' && (
          <Button size="sm" variant="danger" onClick={() => life.stop.mutate(w.operation_id)}>Stop</Button>
        )}
      </div>
      <div className="mt-2 text-xs text-slate-500">
        {formatNumber(w.ops_done)} ops · {w.errors} errors · target {w.params.target_ops_per_sec}/s × {w.params.workers} workers
      </div>
      <div className="mt-3 grid grid-cols-3 gap-2">
        <div><Label>Target ops/sec</Label><Input type="number" value={rate} onChange={(e) => setRate(Number(e.target.value))} /></div>
        <div><Label>Workers</Label><Input type="number" value={workers} onChange={(e) => setWorkers(Number(e.target.value))} /></div>
        <div className="flex items-end"><Button variant="primary" size="sm" onClick={doRate}>Apply</Button></div>
      </div>
    </div>
  );
}
