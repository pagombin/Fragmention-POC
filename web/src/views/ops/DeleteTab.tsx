import { useState } from 'react';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Input, Label, ProgressBar, Select, cx, formatNumber } from '../../components/ui/primitives';
import { Modal } from '../../components/ui/Modal';
import { TargetSelector } from '../../components/TargetSelector';
import { useActiveDeleters, useDeleterLifecycle, useDeleterPreview, useDeleterStart } from '../../api/hooks';
import { useSelection } from '../../stores/selection';
import { useUI } from '../../stores/ui';
import { ApiError } from '../../api/client';
import type { DeletePattern, DeletePreview, DeleterStatus } from '../../api/types';

const PATTERNS: { value: DeletePattern; label: string; help: string }[] = [
  { value: 'random_by_id', label: 'random_by_id', help: 'Random sample of _ids. Candidate set captured up front for stable pause/resume.' },
  { value: 'range_by_field', label: 'range_by_field', help: 'Docs where field < range_before. Server-side filter.' },
  { value: 'ttl_simulated', label: 'ttl_simulated', help: 'Docs older than range_before (defaults to created_at).' },
  { value: 'modulo', label: 'modulo', help: 'Every Nth doc by _id hash. Uniform holes → maximum fragmentation.' },
  { value: 'prefix_by_id', label: 'prefix_by_id', help: 'Docs whose _id starts with prefix (UUID string _ids).' },
];

export function DeleteTab() {
  const sel = useSelection();
  const pushToast = useUI((s) => s.pushToast);
  const previewMut = useDeleterPreview();
  const startMut = useDeleterStart();
  const active = useActiveDeleters();

  const [pattern, setPattern] = useState<DeletePattern>('random_by_id');
  const [ratio, setRatio] = useState(0.3);
  const [field, setField] = useState('created_at');
  const [rangeBefore, setRangeBefore] = useState<string>(new Date().toISOString().slice(0, 16));
  const [modulus, setModulus] = useState(10);
  const [prefix, setPrefix] = useState('00');
  const [seed, setSeed] = useState<number>(() => Math.floor(Math.random() * 1_000_000));
  const [batchSize, setBatchSize] = useState(1000);

  const [preview, setPreview] = useState<DeletePreview | null>(null);
  const [typedConfirm, setTypedConfirm] = useState('');

  const canPreview = sel.selected.size > 0 && (pattern !== 'range_by_field' || field.trim() !== '') && batchSize > 0;

  const buildTargets = () => sel.entries().map((t) => ({
    ...t,
    pattern: {
      kind: pattern,
      ratio: (pattern === 'random_by_id' || pattern === 'modulo') ? ratio : undefined,
      field: pattern === 'range_by_field' ? field : undefined,
      range_before: (pattern === 'range_by_field' || pattern === 'ttl_simulated') ? new Date(rangeBefore).toISOString() : undefined,
      modulus: pattern === 'modulo' ? modulus : undefined,
      prefix: pattern === 'prefix_by_id' ? prefix : undefined,
      seed,
    },
  }));

  const onPreview = async () => {
    try {
      const res = await previewMut.mutateAsync({
        spec: { entries: buildTargets() },
        params: { batch_size: batchSize, max_ratio: 0.95 },
      });
      setPreview(res);
      setTypedConfirm('');
    } catch (err) {
      const msg = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
      pushToast({ kind: 'error', title: 'Preview failed', body: msg });
    }
  };

  const requiredTyped = preview?.requires_typed_confirmation ? 'DELETE' : null;
  const canConfirm = preview != null && !preview.max_ratio_breached && (requiredTyped === null || typedConfirm === requiredTyped);

  const onConfirm = async () => {
    if (!preview) return;
    try {
      const res = await startMut.mutateAsync({ confirmation_token: preview.confirmation_token });
      pushToast({ kind: 'success', title: 'Delete started', body: `operation ${res.operation_id.slice(0, 8)}` });
      setPreview(null);
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
          <CardHeader><CardTitle>Delete pattern</CardTitle></CardHeader>
          <CardBody>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label>Pattern</Label>
                <Select value={pattern} onChange={(e) => setPattern(e.target.value as DeletePattern)}>
                  {PATTERNS.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
                </Select>
                <div className="mt-1 text-xs text-slate-500">{PATTERNS.find((p) => p.value === pattern)?.help}</div>
              </div>
              {(pattern === 'random_by_id' || pattern === 'modulo') && (
                <RatioField ratio={ratio} setRatio={setRatio} />
              )}
              {pattern === 'range_by_field' && (
                <div>
                  <Label>Field</Label>
                  <Input value={field} onChange={(e) => setField(e.target.value)} placeholder="created_at" />
                </div>
              )}
              {(pattern === 'range_by_field' || pattern === 'ttl_simulated') && (
                <div>
                  <Label>Cutoff (older than)</Label>
                  <Input type="datetime-local" value={rangeBefore} onChange={(e) => setRangeBefore(e.target.value)} />
                </div>
              )}
              {pattern === 'modulo' && (
                <div>
                  <Label>Modulus (every Nth)</Label>
                  <Input type="number" min={2} value={modulus} onChange={(e) => setModulus(Math.max(2, Number(e.target.value)))} />
                </div>
              )}
              {pattern === 'prefix_by_id' && (
                <div>
                  <Label>_id hex prefix</Label>
                  <Input value={prefix} onChange={(e) => setPrefix(e.target.value)} placeholder="00" />
                </div>
              )}
              <div>
                <Label>Seed (deterministic)</Label>
                <Input type="number" value={seed} onChange={(e) => setSeed(Number(e.target.value))} />
              </div>
              <div>
                <Label>Batch size</Label>
                <Input type="number" min={1} value={batchSize} onChange={(e) => setBatchSize(Math.max(1, Number(e.target.value)))} />
              </div>
            </div>

            <div className="mt-4 flex items-center gap-2">
              <Button variant="primary" onClick={onPreview} disabled={!canPreview || previewMut.isPending}>
                {previewMut.isPending ? 'Computing preview…' : 'Preview'}
              </Button>
              {!canPreview && <span className="text-xs text-slate-500">Pick at least one collection first.</span>}
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader><CardTitle>Active deletes</CardTitle></CardHeader>
          <CardBody>
            {active.isLoading && <div className="text-slate-500 text-sm">Loading…</div>}
            {active.data?.length === 0 && <div className="text-slate-500 text-sm">No active deletes.</div>}
            <div className="space-y-3">
              {active.data?.map((d) => <DeleterCard key={d.operation_id} d={d} />)}
            </div>
          </CardBody>
        </Card>
      </div>

      <Modal
        open={preview != null}
        title="Confirm delete"
        onClose={() => setPreview(null)}
        width="lg"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPreview(null)}>Cancel</Button>
            <Button
              variant="danger"
              onClick={onConfirm}
              disabled={!canConfirm || startMut.isPending}
              title={preview?.max_ratio_breached ? 'Preview exceeds configured max_ratio' : undefined}
            >
              {startMut.isPending ? 'Starting…' : `Confirm & delete ${preview?.total_matches.toLocaleString() ?? ''}`}
            </Button>
          </>
        }
      >
        {preview && (
          <div className="space-y-3 text-sm">
            <div className="flex items-center justify-between">
              <div>
                Token <code className="mono-sm">{preview.confirmation_token.slice(0, 16)}…</code>
              </div>
              <div className="text-xs text-slate-500">
                expires {new Date(preview.expires_at).toLocaleTimeString()}
              </div>
            </div>
            <div className="rounded border border-slate-200 p-2 dark:border-slate-800">
              <div className="text-xs uppercase tracking-wide text-slate-500 mb-1">Impact</div>
              <div className="font-mono text-sm">
                {formatNumber(preview.total_matches)} documents across {preview.per_collection.length} collection{preview.per_collection.length === 1 ? '' : 's'}
              </div>
              <table className="mt-2 w-full text-xs">
                <thead>
                  <tr className="text-left text-slate-500">
                    <th className="py-0.5">Collection</th>
                    <th className="text-right">Matched</th>
                    <th className="text-right">Total</th>
                    <th className="text-right">Ratio</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.per_collection.map((pc) => {
                    const r = pc.total_documents > 0 ? pc.matched_count / pc.total_documents : 0;
                    return (
                      <tr key={`${pc.database}.${pc.collection}`} className="border-t border-slate-100 dark:border-slate-900">
                        <td className="py-0.5 font-mono">{pc.database}.{pc.collection}</td>
                        <td className="py-0.5 text-right font-mono">{formatNumber(pc.matched_count)}</td>
                        <td className="py-0.5 text-right font-mono">{formatNumber(pc.total_documents)}</td>
                        <td className="py-0.5 text-right font-mono">{(r * 100).toFixed(1)}%</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            {preview.max_ratio_breached && (
              <div className="rounded border border-red-500 bg-red-50 p-2 text-sm text-red-900 dark:bg-red-900/40 dark:text-red-100">
                Preview exceeds configured max_ratio. Refine the pattern and preview again.
              </div>
            )}

            {preview.requires_typed_confirmation && !preview.max_ratio_breached && (
              <div>
                <Label>Type <span className="font-mono">DELETE</span> to confirm</Label>
                <Input value={typedConfirm} onChange={(e) => setTypedConfirm(e.target.value)} />
              </div>
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}

function RatioField({ ratio, setRatio }: { ratio: number; setRatio: (v: number) => void }) {
  return (
    <div>
      <Label>Ratio (0.01 – 0.95)</Label>
      <div className="flex items-center gap-2">
        <input
          type="range"
          min={0.01}
          max={0.95}
          step={0.01}
          value={ratio}
          onChange={(e) => setRatio(Number(e.target.value))}
          className="flex-1"
        />
        <Input
          type="number"
          step={0.01}
          min={0.01}
          max={0.95}
          value={ratio}
          onChange={(e) => setRatio(Math.min(0.95, Math.max(0.01, Number(e.target.value))))}
          className="w-24"
        />
      </div>
    </div>
  );
}

function DeleterCard({ d }: { d: DeleterStatus }) {
  const life = useDeleterLifecycle();
  const progressPct = d.progress && d.progress.length
    ? d.progress.reduce((acc, p) => acc + (p.total_target > 0 ? p.completed_count / p.total_target : 0), 0) / d.progress.length
    : 0;
  return (
    <div className="rounded border border-slate-200 dark:border-slate-800 p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs">{d.operation_id.slice(0, 8)}</span>
          <Badge tone={d.state === 'running' ? 'success' : d.state === 'paused' ? 'warn' : 'neutral'}>{d.state}</Badge>
        </div>
        <div className="flex items-center gap-1">
          {d.state === 'running' && <Button size="sm" onClick={() => life.pause.mutate(d.operation_id)}>Pause</Button>}
          {d.state === 'paused' && <Button size="sm" variant="primary" onClick={() => life.resume.mutate(d.operation_id)}>Resume</Button>}
          {(d.state === 'running' || d.state === 'paused') && <Button size="sm" variant="danger" onClick={() => life.stop.mutate(d.operation_id)}>Stop</Button>}
        </div>
      </div>
      <div className="mt-2 text-xs text-slate-500">
        {d.stats.deleted.toLocaleString()} deleted · {d.stats.batches_ok} batches · {d.stats.batches_failed} failed
      </div>
      <ProgressBar className={cx('mt-2', d.state === 'running' && 'animate-pulse')} value={progressPct} />
      {d.progress && d.progress.length > 0 && (
        <details className="mt-2 text-xs">
          <summary className="cursor-pointer text-slate-500">Per-collection ({d.progress.length})</summary>
          <table className="mt-2 w-full">
            <tbody>
              {d.progress.map((p) => (
                <tr key={p.collection_key} className="border-t border-slate-100 dark:border-slate-900">
                  <td className="py-0.5 font-mono pr-2">{p.collection_key}</td>
                  <td className="py-0.5 text-right pr-2">{formatNumber(p.completed_count)} / {formatNumber(p.total_target)}</td>
                  <td className="py-0.5 w-24"><ProgressBar value={p.completed_count} max={p.total_target} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </details>
      )}
    </div>
  );
}
