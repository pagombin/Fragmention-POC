import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { Badge, Button, ProgressBar, cx, formatBytes } from '../ui/primitives';
import { useUI } from '../../stores/ui';
import {
  useActiveCompacts, useActiveDeleters, useActiveLoaders, useActiveWorkloads,
  useCompactCancel, useDeleterLifecycle, useLoaderLifecycle, useWorkloadLifecycle,
} from '../../api/hooks';
import type { OperationState } from '../../api/types';

// stateTone maps lifecycle state to the badge color used across the panel.
function stateTone(s: OperationState) {
  if (s === 'running') return 'success' as const;
  if (s === 'paused' || s === 'stopping') return 'warn' as const;
  if (s === 'failed' || s === 'interrupted') return 'error' as const;
  return 'neutral' as const;
}

export function LiveOpsPanel() {
  const { liveOpsCollapsed, setLiveOpsCollapsed } = useUI();

  const loaders = useActiveLoaders();
  const deleters = useActiveDeleters();
  const compacts = useActiveCompacts();
  const workloads = useActiveWorkloads();

  const loaderLife = useLoaderLifecycle();
  const deleterLife = useDeleterLifecycle();
  const compactCancel = useCompactCancel();
  const workloadLife = useWorkloadLifecycle();

  const total = useMemo(
    () =>
      (loaders.data?.length ?? 0) +
      (deleters.data?.length ?? 0) +
      (compacts.data?.length ?? 0) +
      (workloads.data?.length ?? 0),
    [loaders.data, deleters.data, compacts.data, workloads.data],
  );

  if (liveOpsCollapsed) {
    return (
      <aside className="flex w-10 shrink-0 flex-col items-center border-l border-slate-200 bg-white py-2 dark:border-slate-800 dark:bg-slate-950">
        <Button variant="ghost" size="sm" onClick={() => setLiveOpsCollapsed(false)} title="Expand live ops">◂</Button>
        <div className="mt-3 text-xs font-mono">{total}</div>
      </aside>
    );
  }

  return (
    <aside className="flex w-72 shrink-0 flex-col border-l border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
      <div className="flex items-center justify-between border-b border-slate-200 px-3 py-2 dark:border-slate-800">
        <div className="text-xs uppercase tracking-wide text-slate-500 dark:text-slate-400">Live ops · {total}</div>
        <Button variant="ghost" size="sm" onClick={() => setLiveOpsCollapsed(true)}>▸</Button>
      </div>
      <div className="flex-1 overflow-y-auto p-2 space-y-2">
        {total === 0 && (
          <div className="rounded border border-dashed border-slate-300 p-3 text-center text-xs text-slate-500 dark:border-slate-700">
            No active operations
          </div>
        )}

        {loaders.data?.map((l) => {
          const completed = l.stats.bytes_inserted;
          return (
            <OpCard
              key={l.operation_id}
              title={`Loader ${l.operation_id.slice(0, 8)}`}
              state={l.state}
              progressLine={`${formatBytes(completed)} written`}
              actions={
                <>
                  {l.state === 'running' && (
                    <Button size="sm" variant="secondary" onClick={() => loaderLife.pause.mutate(l.operation_id)}>Pause</Button>
                  )}
                  {l.state === 'paused' && (
                    <Button size="sm" variant="secondary" onClick={() => loaderLife.resume.mutate(l.operation_id)}>Resume</Button>
                  )}
                  {(l.state === 'running' || l.state === 'paused') && (
                    <Button size="sm" variant="danger" onClick={() => loaderLife.stop.mutate(l.operation_id)}>Stop</Button>
                  )}
                  <Link className="text-xs underline text-brand-500" to="/ops">open</Link>
                </>
              }
            />
          );
        })}

        {deleters.data?.map((d) => (
          <OpCard
            key={d.operation_id}
            title={`Delete ${d.operation_id.slice(0, 8)}`}
            state={d.state}
            progressLine={`${d.stats.deleted.toLocaleString()} deleted`}
            actions={
              <>
                {d.state === 'running' && (
                  <Button size="sm" variant="secondary" onClick={() => deleterLife.pause.mutate(d.operation_id)}>Pause</Button>
                )}
                {d.state === 'paused' && (
                  <Button size="sm" variant="secondary" onClick={() => deleterLife.resume.mutate(d.operation_id)}>Resume</Button>
                )}
                {(d.state === 'running' || d.state === 'paused') && (
                  <Button size="sm" variant="danger" onClick={() => deleterLife.stop.mutate(d.operation_id)}>Stop</Button>
                )}
                <Link className="text-xs underline text-brand-500" to="/ops">open</Link>
              </>
            }
          />
        ))}

        {compacts.data?.map((c) => (
          <OpCard
            key={c.operation_id}
            title={`Compact ${c.operation_id.slice(0, 8)}`}
            state={c.state}
            progressLine={c.current_step || '—'}
            actions={
              <>
                {c.state === 'running' && (
                  <Button size="sm" variant="danger" onClick={() => compactCancel.mutate(c.operation_id)}>Cancel</Button>
                )}
                <Link className="text-xs underline text-brand-500" to="/ops">open</Link>
              </>
            }
          />
        ))}

        {workloads.data?.map((w) => (
          <OpCard
            key={w.operation_id}
            title={`Workload ${w.operation_id.slice(0, 8)}`}
            state={w.state}
            progressLine={`${w.ops_done.toLocaleString()} ops · ${w.errors} errs`}
            actions={
              <>
                {w.state === 'running' && (
                  <Button size="sm" variant="danger" onClick={() => workloadLife.stop.mutate(w.operation_id)}>Stop</Button>
                )}
                <Link className="text-xs underline text-brand-500" to="/ops">open</Link>
              </>
            }
          />
        ))}
      </div>
    </aside>
  );
}

function OpCard({
  title, state, progressLine, actions,
}: {
  title: string;
  state: OperationState;
  progressLine: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="rounded border border-slate-200 bg-slate-50 p-2 dark:border-slate-800 dark:bg-slate-900">
      <div className="flex items-center justify-between">
        <span className="truncate font-mono text-xs">{title}</span>
        <Badge tone={stateTone(state)}>{state}</Badge>
      </div>
      <div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{progressLine}</div>
      <ProgressBar className={cx('mt-2', state === 'running' && 'animate-pulse')} value={state === 'running' ? 0.5 : 1} />
      <div className="mt-2 flex flex-wrap items-center gap-1.5">{actions}</div>
    </div>
  );
}
