import { useMemo, useState } from 'react';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Input, Label, ProgressBar, cx, formatBytes } from '../../components/ui/primitives';
import { TargetSelector } from '../../components/TargetSelector';
import { useActiveLoaders, useLoaderAdjust, useLoaderLifecycle, useStartLoader } from '../../api/hooks';
import { useSelection } from '../../stores/selection';
import { useUI } from '../../stores/ui';
import { ApiError } from '../../api/client';
import type { LoaderStatus } from '../../api/types';

// LoadTab is the spec § 5.7 Operations Console Load tab. It prefills the
// target selection from the shared store and exposes the full lifecycle
// (start/pause/resume/stop/adjust) inline per active operation.

const UNITS: Record<string, number> = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3 };

export function LoadTab() {
  const sel = useSelection();
  const pushToast = useUI((s) => s.pushToast);
  const startMut = useStartLoader();
  const active = useActiveLoaders();

  // Config state (live inside the form)
  const [bytesPer, setBytesPer] = useState('10');
  const [bytesUnit, setBytesUnit] = useState<keyof typeof UNITS>('MB');
  const [workers, setWorkers] = useState(4);
  const [batchSize, setBatchSize] = useState(1000);
  const [docsPerSec, setDocsPerSec] = useState(0);
  const [force, setForce] = useState(true);

  const bytesTarget = useMemo(
    () => Math.max(0, Math.round(Number(bytesPer || '0') * (UNITS[bytesUnit] ?? 1))),
    [bytesPer, bytesUnit],
  );
  const targetCount = sel.selected.size;
  const totalBytes = bytesTarget * targetCount;

  const canStart = targetCount > 0 && bytesTarget > 0 && workers > 0 && batchSize > 0;

  const onStart = async () => {
    if (!canStart) return;
    const entries = sel.entries().map((t) => ({ ...t, bytes_target: bytesTarget }));
    try {
      const res = await startMut.mutateAsync({
        spec: { entries },
        params: {
          workers,
          batch_size: batchSize,
          docs_per_second: docsPerSec || 0,
          force_start: force,
        },
      });
      pushToast({ kind: 'success', title: 'Loader started', body: `operation ${res.operation_id.slice(0, 8)}` });
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
          <CardHeader><CardTitle>Load configuration</CardTitle></CardHeader>
          <CardBody>
            <div className="grid grid-cols-2 gap-3">
              <div title="The loader stops inserting into a collection once it has written approximately this many BSON-encoded bytes. Multiplied across all selected collections.">
                <Label>Bytes target per collection</Label>
                <div className="flex gap-1">
                  <Input type="number" value={bytesPer} onChange={(e) => setBytesPer(e.target.value)} className="flex-1" />
                  <select className="rounded border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 px-1 text-sm" value={bytesUnit} onChange={(e) => setBytesUnit(e.target.value as keyof typeof UNITS)}>
                    {Object.keys(UNITS).map((u) => <option key={u}>{u}</option>)}
                  </select>
                </div>
                <div className="mt-1 text-xs text-slate-500">
                  {formatBytes(bytesTarget)} × {targetCount} collections = {formatBytes(totalBytes)} total
                </div>
              </div>

              <div title="Number of concurrent goroutines pushing batches. Higher = faster fill, more cluster load.">
                <Label>Workers</Label>
                <Input type="number" min={1} value={workers} onChange={(e) => setWorkers(Math.max(1, Number(e.target.value)))} />
              </div>

              <div title="Number of documents per InsertMany call. Larger batches are more efficient but use more memory and longer per-batch round-trips.">
                <Label>Batch size</Label>
                <Input type="number" min={1} value={batchSize} onChange={(e) => setBatchSize(Math.max(1, Number(e.target.value)))} />
              </div>

              <div title="Total documents per second across all workers. Set 0 to let workers run as fast as the cluster allows.">
                <Label>Docs/sec rate limit (0 = unlimited)</Label>
                <Input type="number" min={0} value={docsPerSec} onChange={(e) => setDocsPerSec(Math.max(0, Number(e.target.value)))} />
              </div>

              <div className="col-span-2 flex items-center gap-2" title="Bypass the headroom check that refuses a load when free disk < target × (1+headroom). Useful on managed clusters where fsTotalSize isn't reported.">
                <input id="force" type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />
                <label htmlFor="force" className="text-sm">Skip pre-flight storage check (force start)</label>
              </div>
            </div>

            <div className="mt-4 flex items-center gap-2">
              <Button variant="primary" onClick={onStart} disabled={!canStart || startMut.isPending}>
                {startMut.isPending ? 'Starting…' : `Start load (${targetCount} targets)`}
              </Button>
              {!canStart && <span className="text-xs text-slate-500">Pick at least one collection and set bytes_target.</span>}
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader><CardTitle>Active loaders</CardTitle></CardHeader>
          <CardBody>
            {active.isLoading && <div className="text-slate-500 text-sm">Loading…</div>}
            {active.data?.length === 0 && <div className="text-slate-500 text-sm">No active loaders.</div>}
            <div className="space-y-3">
              {active.data?.map((l) => <LoaderCard key={l.operation_id} l={l} />)}
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
}

function LoaderCard({ l }: { l: LoaderStatus }) {
  const life = useLoaderLifecycle();
  const adjust = useLoaderAdjust();
  const pushToast = useUI((s) => s.pushToast);
  const [open, setOpen] = useState(false);
  const [workers, setWorkers] = useState(l.params.workers ?? 4);
  const [batch, setBatch] = useState(l.params.batch_size);
  const [rate, setRate] = useState(l.params.docs_per_second ?? 0);

  const progressPct = l.progress && l.progress.length
    ? l.progress.reduce((acc, p) => acc + (p.total_target > 0 ? p.bytes_processed / p.total_target : 0), 0) / l.progress.length
    : 0;

  const doAdjust = async () => {
    try {
      await adjust.mutateAsync({
        id: l.operation_id,
        params: { ...l.params, workers, batch_size: batch, docs_per_second: rate },
      });
      pushToast({ kind: 'success', title: 'Params updated' });
    } catch (err) {
      pushToast({ kind: 'error', title: 'Adjust failed', body: String(err) });
    }
  };

  return (
    <div className="rounded border border-slate-200 dark:border-slate-800 p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs">{l.operation_id.slice(0, 8)}</span>
          <Badge tone={l.state === 'running' ? 'success' : l.state === 'paused' ? 'warn' : 'neutral'}>{l.state}</Badge>
        </div>
        <div className="flex items-center gap-1">
          {l.state === 'running' && <Button size="sm" onClick={() => life.pause.mutate(l.operation_id)}>Pause</Button>}
          {l.state === 'paused' && <Button size="sm" variant="primary" onClick={() => life.resume.mutate(l.operation_id)}>Resume</Button>}
          {(l.state === 'running' || l.state === 'paused') && <Button size="sm" variant="danger" onClick={() => life.stop.mutate(l.operation_id)}>Stop</Button>}
          <Button size="sm" variant="ghost" onClick={() => setOpen((o) => !o)}>{open ? 'Hide params' : 'Adjust'}</Button>
        </div>
      </div>
      <div className="mt-2 grid grid-cols-3 gap-2 text-xs text-slate-500">
        <span>{l.stats.docs_inserted.toLocaleString()} docs</span>
        <span>{formatBytes(l.stats.bytes_inserted)} written</span>
        <span>{l.stats.batches_ok} batches · {l.stats.batches_failed} failed</span>
      </div>
      <ProgressBar className={cx('mt-2', l.state === 'running' && 'animate-pulse')} value={progressPct} max={1} />
      {open && (
        <div className="mt-3 grid grid-cols-3 gap-2">
          <div><Label>Workers</Label><Input type="number" value={workers} onChange={(e) => setWorkers(Number(e.target.value))} /></div>
          <div><Label>Batch</Label><Input type="number" value={batch} onChange={(e) => setBatch(Number(e.target.value))} /></div>
          <div><Label>Docs/sec</Label><Input type="number" value={rate} onChange={(e) => setRate(Number(e.target.value))} /></div>
          <div className="col-span-3">
            <Button size="sm" variant="primary" onClick={doAdjust} disabled={adjust.isPending}>
              {adjust.isPending ? 'Applying…' : 'Apply adjustment'}
            </Button>
          </div>
        </div>
      )}
      {l.progress && l.progress.length > 0 && (
        <details className="mt-2 text-xs">
          <summary className="cursor-pointer text-slate-500">Per-collection progress ({l.progress.length})</summary>
          <table className="mt-2 w-full">
            <tbody>
              {l.progress.map((p) => (
                <tr key={p.collection_key} className="border-t border-slate-100 dark:border-slate-900">
                  <td className="py-0.5 font-mono pr-2">{p.collection_key}</td>
                  <td className="py-0.5 text-right pr-2">{formatBytes(p.bytes_processed)}/{formatBytes(p.total_target)}</td>
                  <td className="py-0.5 w-24"><ProgressBar value={p.bytes_processed} max={p.total_target} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      )}
    </div>
  );
}
