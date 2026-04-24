import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Page } from './ClusterOverview';
import { Button, Card, CardBody, CardHeader, CardTitle, Input, Label, Select, cx } from '../components/ui/primitives';
import { useCreateRun } from '../api/hooks';
import { useUI } from '../stores/ui';

// NewRunWizard is a multi-step form that creates a run record containing
// the full configuration. The actual load/delete/compact/workload phases
// are triggered from the Operations Console using the created run_id - we
// keep this lean rather than re-implementing every step here.

const STEPS = ['Confirm topology', 'Load config', 'Delete config', 'Reclaim', 'Workload', 'Review'];

export function NewRunWizard() {
  const nav = useNavigate();
  const create = useCreateRun();
  const pushToast = useUI((s) => s.pushToast);
  const [step, setStep] = useState(0);

  const [name, setName] = useState(`run-${new Date().toISOString().slice(0, 16).replace(/[:T-]/g, '')}`);
  const [bytesTargetMB, setBytesTargetMB] = useState(100);
  const [pattern, setPattern] = useState('random_by_id');
  const [ratio, setRatio] = useState(0.3);
  const [reclaim, setReclaim] = useState<'compact' | 'initial_sync' | 'none'>('compact');
  const [workload, setWorkload] = useState(true);
  const [opsPerSec, setOpsPerSec] = useState(100);
  const [notes, setNotes] = useState('');

  const config = {
    load: { bytes_target_mb: bytesTargetMB },
    delete: { pattern, ratio },
    reclaim,
    workload: { enabled: workload, target_ops_per_sec: opsPerSec },
  };

  const onCreate = async () => {
    try {
      const run = await create.mutateAsync({ name, config, notes });
      pushToast({ kind: 'success', title: 'Run created', body: name });
      nav(`/runs/${run.id}`);
    } catch (err) {
      pushToast({ kind: 'error', title: 'Create failed', body: String(err) });
    }
  };

  return (
    <Page>
      <Card>
        <CardHeader>
          <CardTitle>New run wizard</CardTitle>
        </CardHeader>
        <CardBody>
          <div className="mb-3 flex items-center gap-2">
            {STEPS.map((s, i) => (
              <div key={s} className={cx('flex items-center gap-1', i === step ? 'text-brand-500' : 'text-slate-500')}>
                <span className={cx('inline-flex h-5 w-5 items-center justify-center rounded-full text-xs',
                  i === step ? 'bg-brand-600 text-white' : 'bg-slate-200 text-slate-700 dark:bg-slate-800 dark:text-slate-300')}>{i + 1}</span>
                <span className="text-xs">{s}</span>
                {i < STEPS.length - 1 && <span className="text-slate-400">·</span>}
              </div>
            ))}
          </div>

          {step === 0 && (
            <div className="space-y-2 text-sm">
              <div>Confirm your cluster topology and credentials in the Cluster Overview before proceeding. Runs created here record configuration only; phases execute through the Operations Console.</div>
              <div>
                <Label>Run name</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} />
              </div>
            </div>
          )}

          {step === 1 && (
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label>Bytes target per collection (MB)</Label>
                <Input type="number" min={1} value={bytesTargetMB} onChange={(e) => setBytesTargetMB(Math.max(1, Number(e.target.value)))} />
              </div>
            </div>
          )}

          {step === 2 && (
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label>Deletion pattern</Label>
                <Select value={pattern} onChange={(e) => setPattern(e.target.value)}>
                  {['random_by_id', 'range_by_field', 'ttl_simulated', 'modulo', 'prefix_by_id'].map((p) => <option key={p}>{p}</option>)}
                </Select>
              </div>
              <div>
                <Label>Ratio</Label>
                <Input type="number" step={0.05} min={0.01} max={0.95} value={ratio} onChange={(e) => setRatio(Number(e.target.value))} />
              </div>
            </div>
          )}

          {step === 3 && (
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label>Reclaim strategy</Label>
                <Select value={reclaim} onChange={(e) => setReclaim(e.target.value as typeof reclaim)}>
                  <option value="compact">compact</option>
                  <option value="initial_sync">external initial sync</option>
                  <option value="none">none</option>
                </Select>
                <div className="mt-1 text-xs text-slate-500">
                  {reclaim === 'initial_sync' && 'You will drive the initial sync externally and use the Initial Sync Companion view.'}
                  {reclaim === 'compact' && 'Compact will run via the orchestrator when you launch the phase from the Operations Console.'}
                </div>
              </div>
            </div>
          )}

          {step === 4 && (
            <div className="grid grid-cols-2 gap-3">
              <div className="flex items-center gap-2">
                <input id="wl" type="checkbox" checked={workload} onChange={(e) => setWorkload(e.target.checked)} />
                <label htmlFor="wl" className="text-sm">Run workload during reclaim</label>
              </div>
              <div>
                <Label>Target ops/sec</Label>
                <Input type="number" min={0} value={opsPerSec} onChange={(e) => setOpsPerSec(Math.max(0, Number(e.target.value)))} />
              </div>
            </div>
          )}

          {step === 5 && (
            <div className="space-y-3">
              <div>
                <Label>Notes</Label>
                <Input value={notes} onChange={(e) => setNotes(e.target.value)} placeholder="Run description, hypothesis, etc." />
              </div>
              <pre className="rounded bg-slate-100 dark:bg-slate-900 p-3 text-xs overflow-auto">{JSON.stringify({ name, notes, config }, null, 2)}</pre>
            </div>
          )}

          <div className="mt-4 flex items-center justify-between">
            <Button variant="ghost" onClick={() => setStep((s) => Math.max(0, s - 1))} disabled={step === 0}>Back</Button>
            {step < STEPS.length - 1 ? (
              <Button variant="primary" onClick={() => setStep((s) => Math.min(STEPS.length - 1, s + 1))}>Next</Button>
            ) : (
              <Button variant="primary" onClick={onCreate} disabled={create.isPending}>
                {create.isPending ? 'Creating…' : 'Create run'}
              </Button>
            )}
          </div>
        </CardBody>
      </Card>
    </Page>
  );
}
