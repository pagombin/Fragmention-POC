import { useMemo, useState } from 'react';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Input, Label, Select, cx } from '../../components/ui/primitives';
import { Modal } from '../../components/ui/Modal';
import { TargetSelector } from '../../components/TargetSelector';
import { useActiveCompacts, useCompactCancel, useCompactPreview, useCompactStart, useDatabases } from '../../api/hooks';
import { useSelection } from '../../stores/selection';
import { useUI } from '../../stores/ui';
import { ApiError } from '../../api/client';
import type { CompactPreview, CompactScope, CompactStatus } from '../../api/types';

type ScopeKind = CompactScope['kind'];

export function CompactTab() {
  const sel = useSelection();
  const dbs = useDatabases();
  const pushToast = useUI((s) => s.pushToast);
  const previewMut = useCompactPreview();
  const startMut = useCompactStart();
  const active = useActiveCompacts();

  const [kind, setKind] = useState<ScopeKind>('cluster');
  const [dbChoice, setDbChoice] = useState<string>('');
  const [maxLagS, setMaxLagS] = useState(10);
  const [stepdownS, setStepdownS] = useState(60);
  const [validate, setValidate] = useState(false);
  const [preview, setPreview] = useState<CompactPreview | null>(null);

  const scope: CompactScope = useMemo(() => {
    switch (kind) {
      case 'cluster':
        return { kind: 'cluster' };
      case 'databases':
        return { kind: 'databases', databases: dbChoice ? [dbChoice] : [] };
      case 'collections':
        return { kind: 'collections', collections: sel.entries() };
    }
  }, [kind, dbChoice, sel.selected]);

  const canPreview = (kind === 'cluster') ||
    (kind === 'databases' && dbChoice !== '') ||
    (kind === 'collections' && sel.selected.size > 0);

  const buildParams = () => ({
    max_replication_lag_ns: maxLagS * 1_000_000_000,
    stepdown_wait_timeout_ns: stepdownS * 1_000_000_000,
    validate_after_compact: validate,
  });

  const onPreview = async () => {
    try {
      const res = await previewMut.mutateAsync({ scope, params: buildParams() });
      setPreview(res);
    } catch (err) {
      const msg = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
      pushToast({ kind: 'error', title: 'Preview failed', body: msg });
    }
  };

  const onStart = async () => {
    try {
      const res = await startMut.mutateAsync({ scope, params: buildParams() });
      pushToast({ kind: 'success', title: 'Compact started', body: `operation ${res.operation_id.slice(0, 8)}` });
      setPreview(null);
    } catch (err) {
      const msg = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
      pushToast({ kind: 'error', title: 'Start failed', body: msg });
    }
  };

  return (
    <div className="grid grid-cols-1 xl:grid-cols-[2fr_3fr] gap-4">
      {kind === 'collections' ? <TargetSelector /> : <Card><CardBody className="text-sm text-slate-500">Scope is {kind}; target selector is not used.</CardBody></Card>}

      <div className="space-y-4">
        <Card>
          <CardHeader><CardTitle>Compact scope</CardTitle></CardHeader>
          <CardBody>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label>Scope kind</Label>
                <Select value={kind} onChange={(e) => setKind(e.target.value as ScopeKind)}>
                  <option value="cluster">cluster (all user databases)</option>
                  <option value="databases">database(s)</option>
                  <option value="collections">specific collections</option>
                </Select>
              </div>
              {kind === 'databases' && (
                <div>
                  <Label>Database</Label>
                  <Select value={dbChoice} onChange={(e) => setDbChoice(e.target.value)}>
                    <option value="">—</option>
                    {dbs.data?.map((d) => <option key={d.name} value={d.name}>{d.name}</option>)}
                  </Select>
                </div>
              )}
              <div>
                <Label>Max replication lag (seconds)</Label>
                <Input type="number" min={0} value={maxLagS} onChange={(e) => setMaxLagS(Math.max(0, Number(e.target.value)))} />
              </div>
              <div>
                <Label>Stepdown wait timeout (seconds)</Label>
                <Input type="number" min={1} value={stepdownS} onChange={(e) => setStepdownS(Math.max(1, Number(e.target.value)))} />
              </div>
              <div className="col-span-2 flex items-center gap-2">
                <input id="validate" type="checkbox" checked={validate} onChange={(e) => setValidate(e.target.checked)} />
                <label htmlFor="validate" className="text-sm">Run validate after compact (expensive)</label>
              </div>
            </div>
            <div className="mt-4 flex items-center gap-2">
              <Button variant="primary" onClick={onPreview} disabled={!canPreview || previewMut.isPending}>
                {previewMut.isPending ? 'Computing plan…' : 'Preview plan'}
              </Button>
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader><CardTitle>Active compacts</CardTitle></CardHeader>
          <CardBody>
            {active.isLoading && <div className="text-slate-500 text-sm">Loading…</div>}
            {active.data?.length === 0 && <div className="text-slate-500 text-sm">No active compacts.</div>}
            <div className="space-y-3">
              {active.data?.map((c) => <CompactCard key={c.operation_id} c={c} />)}
            </div>
          </CardBody>
        </Card>
      </div>

      <Modal
        open={preview != null}
        onClose={() => setPreview(null)}
        title="Planned compact execution"
        width="lg"
        footer={
          <>
            <Button variant="ghost" onClick={() => setPreview(null)}>Cancel</Button>
            <Button variant="primary" onClick={onStart} disabled={startMut.isPending}>
              {startMut.isPending ? 'Starting…' : 'Start compact'}
            </Button>
          </>
        }
      >
        {preview && (
          <div className="space-y-3 text-sm">
            <div className="flex items-center gap-2">
              <Badge tone={preview.mode === 'rolling' ? 'info' : 'neutral'}>{preview.mode}</Badge>
              <span className="text-slate-500">
                {preview.total_collections} collection{preview.total_collections === 1 ? '' : 's'}
                · ~{Math.round(preview.estimated_duration_ns / 1_000_000_000)}s estimated
              </span>
            </div>
            {preview.warnings && preview.warnings.length > 0 && (
              <div className="rounded border border-yellow-500 bg-yellow-50 p-2 text-sm text-yellow-900 dark:bg-yellow-900/30 dark:text-yellow-100">
                <div className="font-semibold">Warnings</div>
                <ul className="list-disc list-inside">{preview.warnings.map((w, i) => <li key={i}>{w}</li>)}</ul>
              </div>
            )}
            <div>
              <div className="mb-1 text-xs uppercase tracking-wide text-slate-500">Execution order</div>
              <ol className="space-y-1">
                {(preview.execution_order ?? []).map((m, idx) => (
                  <li key={idx} className="flex items-start gap-2 rounded border border-slate-200 p-2 text-xs dark:border-slate-800">
                    <span className="font-mono text-slate-400 w-4 text-right">{idx + 1}.</span>
                    <div className="flex-1">
                      <div className="flex items-center gap-2">
                        <Badge tone={m.role === 'primary' ? 'warn' : 'info'}>{m.role}</Badge>
                        <span className="font-mono">{m.member}</span>
                        {m.requires_stepdown && <Badge tone="warn">stepdown required</Badge>}
                      </div>
                      <div className="mt-1 text-slate-500">{m.collections.length} collection{m.collections.length === 1 ? '' : 's'}</div>
                    </div>
                  </li>
                ))}
              </ol>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}

function CompactCard({ c }: { c: CompactStatus }) {
  const cancel = useCompactCancel();
  return (
    <div className="rounded border border-slate-200 dark:border-slate-800 p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs">{c.operation_id.slice(0, 8)}</span>
          <Badge tone={c.state === 'running' ? 'success' : c.state === 'stopping' ? 'warn' : 'neutral'}>{c.state}</Badge>
          {c.current_step && <span className="font-mono text-xs text-slate-500">{c.current_step}</span>}
        </div>
        {(c.state === 'running') && (
          <Button size="sm" variant="danger" onClick={() => cancel.mutate(c.operation_id)}>Cancel</Button>
        )}
      </div>
      <div className={cx('mt-2 h-1.5 w-full rounded bg-slate-200 dark:bg-slate-800 overflow-hidden', c.state === 'running' && 'animate-pulse')}>
        <div className="h-full w-1/2 bg-brand-500" />
      </div>
    </div>
  );
}
