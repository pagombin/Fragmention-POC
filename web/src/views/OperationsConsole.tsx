import { useSearchParams } from 'react-router-dom';
import { Page } from './ClusterOverview';
import { Card, CardBody, cx } from '../components/ui/primitives';
import { LoadTab } from './ops/LoadTab';
import { DeleteTab } from './ops/DeleteTab';
import { CompactTab } from './ops/CompactTab';
import { WorkloadTab } from './ops/WorkloadTab';
import { SnapshotsTab } from './ops/SnapshotsTab';

const TABS = [
  { key: 'load', label: 'Load' },
  { key: 'delete', label: 'Delete' },
  { key: 'compact', label: 'Compact' },
  { key: 'workload', label: 'Workload' },
  { key: 'snapshots', label: 'Snapshots' },
];

export function OperationsConsole() {
  const [params, setParams] = useSearchParams();
  const active = params.get('tab') ?? 'load';

  const go = (tab: string) => {
    const next = new URLSearchParams(params);
    next.set('tab', tab);
    setParams(next, { replace: true });
  };

  return (
    <Page>
      <div className="mb-3 flex gap-1 border-b border-slate-200 dark:border-slate-800">
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => go(t.key)}
            className={cx(
              'px-3 py-1.5 text-sm border-b-2 -mb-px',
              active === t.key
                ? 'border-brand-500 text-brand-600 dark:text-brand-300'
                : 'border-transparent text-slate-500 hover:text-slate-700 dark:hover:text-slate-300',
            )}
          >
            {t.label}
          </button>
        ))}
      </div>
      {active === 'load' && <LoadTab />}
      {active === 'delete' && <DeleteTab />}
      {active === 'compact' && <CompactTab />}
      {active === 'workload' && <WorkloadTab />}
      {active === 'snapshots' && <SnapshotsTab />}
      {!['load','delete','compact','workload','snapshots'].includes(active) && (
        <Card><CardBody>Unknown tab.</CardBody></Card>
      )}
    </Page>
  );
}
