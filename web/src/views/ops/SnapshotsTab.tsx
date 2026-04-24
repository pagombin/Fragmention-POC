import { useState } from 'react';
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, Input, Label, Table, Td, Th, formatBytes, formatNumber } from '../../components/ui/primitives';
import { useCompareSnapshots, useSnapshots, useTakeSnapshot } from '../../api/hooks';
import { useUI } from '../../stores/ui';
import { ApiError } from '../../api/client';

export function SnapshotsTab() {
  const list = useSnapshots();
  const take = useTakeSnapshot();
  const pushToast = useUI((s) => s.pushToast);

  const [label, setLabel] = useState('baseline');
  const [note, setNote] = useState('');
  const [a, setA] = useState<string | null>(null);
  const [b, setB] = useState<string | null>(null);

  const compare = useCompareSnapshots(a, b);

  const onTake = async () => {
    try {
      await take.mutateAsync({ label, note });
      pushToast({ kind: 'success', title: 'Snapshot taken', body: label });
      setNote('');
    } catch (err) {
      const msg = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
      pushToast({ kind: 'error', title: 'Snapshot failed', body: msg });
    }
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader><CardTitle>Take snapshot</CardTitle></CardHeader>
        <CardBody>
          <div className="grid grid-cols-3 gap-3">
            <div><Label>Label</Label><Input value={label} onChange={(e) => setLabel(e.target.value)} /></div>
            <div className="col-span-2"><Label>Note (optional)</Label><Input value={note} onChange={(e) => setNote(e.target.value)} /></div>
          </div>
          <div className="mt-4">
            <Button variant="primary" onClick={onTake} disabled={take.isPending || !label}>
              {take.isPending ? 'Taking…' : 'Take snapshot'}
            </Button>
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardHeader className="flex items-center justify-between">
          <CardTitle>Snapshots</CardTitle>
          <span className="text-xs text-slate-500">Select two to compare</span>
        </CardHeader>
        <CardBody>
          {list.isLoading && <div className="text-slate-500 text-sm">Loading…</div>}
          {list.data?.length === 0 && <div className="text-slate-500 text-sm">No snapshots yet.</div>}
          {list.data && list.data.length > 0 && (
            <Table>
              <thead>
                <tr>
                  <Th>Label</Th><Th>Taken</Th><Th>Run</Th><Th>Note</Th><Th className="w-32 text-right">Compare</Th>
                </tr>
              </thead>
              <tbody>
                {list.data.map((s) => (
                  <tr key={s.id}>
                    <Td><Badge>{s.label}</Badge></Td>
                    <Td className="font-mono text-xs">{new Date(s.taken_at).toLocaleString()}</Td>
                    <Td className="font-mono text-xs">{s.run_id ? s.run_id.slice(0, 8) : '—'}</Td>
                    <Td className="text-xs">{s.note}</Td>
                    <Td className="text-right">
                      <div className="inline-flex gap-1">
                        <Button size="sm" variant={a === s.id ? 'primary' : 'ghost'} onClick={() => setA(s.id === a ? null : s.id)}>A</Button>
                        <Button size="sm" variant={b === s.id ? 'primary' : 'ghost'} onClick={() => setB(s.id === b ? null : s.id)}>B</Button>
                      </div>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </CardBody>
      </Card>

      {a && b && (
        <Card>
          <CardHeader><CardTitle>Diff A → B</CardTitle></CardHeader>
          <CardBody>
            {compare.isLoading && <div className="text-slate-500 text-sm">Computing diff…</div>}
            {compare.isError && <div className="text-red-500 text-sm">Failed: {String(compare.error)}</div>}
            {compare.data && (
              <>
                <div className="mb-2 flex items-center gap-3 text-xs text-slate-500">
                  <div>A · <Badge>{compare.data.a.label}</Badge> · {new Date(compare.data.a.taken_at).toLocaleString()}</div>
                  <div>B · <Badge>{compare.data.b.label}</Badge> · {new Date(compare.data.b.taken_at).toLocaleString()}</div>
                </div>
                <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
                  <div>
                    <h4 className="text-xs uppercase text-slate-500 mb-1">Databases</h4>
                    <Table>
                      <thead><tr><Th>Name</Th><Th className="text-right">Storage Δ</Th><Th className="text-right">Data Δ</Th></tr></thead>
                      <tbody>
                        {compare.data.diff.databases?.map((d) => (
                          <tr key={d.name}>
                            <Td className="font-mono text-xs">{d.name}</Td>
                            <Td className="text-right font-mono">{formatBytes(d.storage_delta)}</Td>
                            <Td className="text-right font-mono">{formatBytes(d.data_delta)}</Td>
                          </tr>
                        ))}
                      </tbody>
                    </Table>
                  </div>
                  <div>
                    <h4 className="text-xs uppercase text-slate-500 mb-1">Collections</h4>
                    <Table>
                      <thead>
                        <tr>
                          <Th>Scope</Th>
                          <Th className="text-right">Storage Δ</Th>
                          <Th className="text-right">Free Δ</Th>
                          <Th className="text-right">Count Δ</Th>
                          <Th className="text-right">Frag before → after</Th>
                        </tr>
                      </thead>
                      <tbody>
                        {compare.data.diff.collections?.map((c) => (
                          <tr key={`${c.database}.${c.collection}`}>
                            <Td className="font-mono text-xs">{c.database}.{c.collection}</Td>
                            <Td className="text-right font-mono">{formatBytes(c.storage_delta)}</Td>
                            <Td className="text-right font-mono">{formatBytes(c.free_storage_delta)}</Td>
                            <Td className="text-right font-mono">{formatNumber(c.count_delta)}</Td>
                            <Td className="text-right font-mono text-xs">
                              {(c.fragmentation_before * 100).toFixed(1)}% → {(c.fragmentation_after * 100).toFixed(1)}%
                            </Td>
                          </tr>
                        ))}
                      </tbody>
                    </Table>
                  </div>
                </div>
              </>
            )}
          </CardBody>
        </Card>
      )}
    </div>
  );
}
