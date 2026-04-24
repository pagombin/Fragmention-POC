import { useMemo, useState } from 'react';
import { Page } from './ClusterOverview';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Table, Td, Th, formatBytes, formatNumber } from '../components/ui/primitives';
import { useCompareSnapshots, useSnapshots, useTakeSnapshot, useTopology } from '../api/hooks';
import { useUI } from '../stores/ui';
import { ApiError } from '../api/client';

// Dedicated no-think workflow for the external initial-sync flow (spec
// § 5.7 view 8). The view refuses to start on single-node topologies and
// expects the operator to run the initial sync on the cluster themselves
// between the two snapshot taps.

export function InitialSyncCompanion() {
  const topology = useTopology();
  const snaps = useSnapshots();
  const take = useTakeSnapshot();
  const pushToast = useUI((s) => s.pushToast);

  const [preID, setPreID] = useState<string | null>(null);
  const [postID, setPostID] = useState<string | null>(null);
  const diff = useCompareSnapshots(preID, postID);

  const singleNode = topology.data?.topology.kind === 'standalone';

  const preLabel = 'pre_initial_sync';
  const postLabel = 'post_initial_sync';

  const takeLabel = async (label: string, setter: (id: string) => void) => {
    try {
      const res = await take.mutateAsync({ label });
      setter(res.id);
      pushToast({ kind: 'success', title: `Snapshot ${label} taken` });
    } catch (err) {
      const msg = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
      pushToast({ kind: 'error', title: 'Snapshot failed', body: msg });
    }
  };

  const totalReclaim = useMemo(() => {
    if (!diff.data) return 0;
    return (diff.data.diff.collections ?? []).reduce((acc, c) => acc - c.storage_delta, 0);
  }, [diff.data]);

  return (
    <Page>
      <Card>
        <CardHeader><CardTitle>Initial Sync Companion</CardTitle></CardHeader>
        <CardBody>
          {singleNode && (
            <div className="rounded border border-yellow-500 bg-yellow-50 p-2 text-sm text-yellow-900 dark:bg-yellow-900/30 dark:text-yellow-100 mb-3">
              This tool requires a replica set. Single-node topologies don't support initial sync.
            </div>
          )}
          <ol className="space-y-3">
            <li className="rounded border border-slate-200 p-3 dark:border-slate-800">
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-xs uppercase text-slate-500">Step 1</div>
                  <div className="text-sm">Capture a <code className="mono-sm">{preLabel}</code> snapshot.</div>
                </div>
                <div className="flex items-center gap-2">
                  {preID ? <Badge tone="success">{preID.slice(0, 8)}</Badge> : <Badge>—</Badge>}
                  <Button variant="primary" disabled={singleNode || take.isPending} onClick={() => takeLabel(preLabel, setPreID)}>
                    Take pre-sync snapshot
                  </Button>
                </div>
              </div>
            </li>

            <li className="rounded border border-slate-200 p-3 dark:border-slate-800">
              <div className="text-xs uppercase text-slate-500">Step 2</div>
              <div className="text-sm">
                Perform the initial sync externally (stop the target <code className="mono-sm">mongod</code>, wipe dbPath, restart, wait for <code className="mono-sm">SECONDARY</code>). The collector will auto-detect the <code className="mono-sm">STARTUP2</code> transition.
              </div>
            </li>

            <li className="rounded border border-slate-200 p-3 dark:border-slate-800">
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-xs uppercase text-slate-500">Step 3</div>
                  <div className="text-sm">When the synced member is back to <code className="mono-sm">SECONDARY</code>, capture a <code className="mono-sm">{postLabel}</code> snapshot.</div>
                </div>
                <div className="flex items-center gap-2">
                  {postID ? <Badge tone="success">{postID.slice(0, 8)}</Badge> : <Badge>—</Badge>}
                  <Button variant="primary" disabled={singleNode || take.isPending} onClick={() => takeLabel(postLabel, setPostID)}>
                    Take post-sync snapshot
                  </Button>
                </div>
              </div>
            </li>
          </ol>
        </CardBody>
      </Card>

      {preID && postID && (
        <Card className="mt-4">
          <CardHeader><CardTitle>Diff pre → post</CardTitle></CardHeader>
          <CardBody>
            {diff.isLoading && <div className="text-slate-500 text-sm">Computing diff…</div>}
            {diff.data && (
              <>
                <div className="mb-3 text-sm">
                  Total reclaim: <strong>{formatBytes(totalReclaim)}</strong> across {diff.data.diff.collections?.length ?? 0} collection{diff.data.diff.collections?.length === 1 ? '' : 's'}.
                </div>
                <Table>
                  <thead>
                    <tr>
                      <Th>Collection</Th>
                      <Th className="text-right">Storage before</Th>
                      <Th className="text-right">Storage after</Th>
                      <Th className="text-right">Reclaimed</Th>
                      <Th className="text-right">Frag before → after</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {diff.data.diff.collections?.map((c) => {
                      const reclaim = -c.storage_delta;
                      return (
                        <tr key={`${c.database}.${c.collection}`}>
                          <Td className="font-mono text-xs">{c.database}.{c.collection}</Td>
                          <Td className="text-right font-mono">{formatBytes(c.storage_before)}</Td>
                          <Td className="text-right font-mono">{formatBytes(c.storage_after)}</Td>
                          <Td className={`text-right font-mono ${reclaim > 0 ? 'text-green-600 dark:text-green-400' : ''}`}>{formatBytes(reclaim)}</Td>
                          <Td className="text-right font-mono text-xs">
                            {(c.fragmentation_before * 100).toFixed(1)}% → {(c.fragmentation_after * 100).toFixed(1)}%
                          </Td>
                        </tr>
                      );
                    })}
                  </tbody>
                </Table>
              </>
            )}
          </CardBody>
        </Card>
      )}

      <Card className="mt-4">
        <CardHeader><CardTitle>Recent snapshots</CardTitle></CardHeader>
        <CardBody>
          <ul className="space-y-1 text-sm">
            {snaps.data?.filter((s) => s.label.includes('initial_sync')).map((s) => (
              <li key={s.id} className="flex items-center justify-between">
                <span className="font-mono text-xs">{s.id.slice(0, 8)} · {s.label}</span>
                <span className="text-xs text-slate-500">{new Date(s.taken_at).toLocaleString()}</span>
              </li>
            ))}
            {snaps.data?.filter((s) => s.label.includes('initial_sync')).length === 0 && (
              <li className="text-xs text-slate-500">No initial-sync snapshots yet.</li>
            )}
          </ul>
          {/* formatNumber kept imported for future use */}
          <span className="hidden">{formatNumber(0)}</span>
        </CardBody>
      </Card>
    </Page>
  );
}
