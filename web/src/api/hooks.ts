import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from './client';
import type {
  CollectionSummary, CompactPreview, CompactScope, CompactStatus, DatabaseSummary,
  DeletePreview, DeleteTarget, DeleterStatus, EventRow, LoaderStatus, LoaderParams,
  LoaderTarget, MetricSample, Run, ServerInfo, Snapshot, Topology, WorkloadParams,
  WorkloadSpec, WorkloadStatus,
} from './types';

/* ---- cluster ---- */

export function useTopology() {
  return useQuery({
    queryKey: ['cluster', 'topology'],
    queryFn: () => api<{ topology: Topology; server_info: ServerInfo; redacted_uri: string; is_srv: boolean }>(
      '/api/v1/cluster/topology',
    ),
    refetchInterval: 5_000,
  });
}

export function useDatabases() {
  return useQuery({
    queryKey: ['cluster', 'databases'],
    queryFn: () => api<DatabaseSummary[]>('/api/v1/cluster/databases'),
    refetchInterval: 10_000,
  });
}

export function useCollections(db: string | null) {
  return useQuery({
    queryKey: ['cluster', 'collections', db],
    queryFn: () => api<CollectionSummary[]>(`/api/v1/cluster/databases/${encodeURIComponent(db!)}/collections`),
    enabled: !!db,
    refetchInterval: 10_000,
  });
}

/* ---- loader ---- */

export function useActiveLoaders() {
  return useQuery({
    queryKey: ['loader', 'active'],
    queryFn: () => api<LoaderStatus[]>('/api/v1/loader/active'),
    refetchInterval: 2_000,
  });
}

export function useLoader(id: string | null) {
  return useQuery({
    queryKey: ['loader', id],
    queryFn: () => api<LoaderStatus>(`/api/v1/loader/${id}`),
    enabled: !!id,
    refetchInterval: 2_000,
  });
}

export function useStartLoader() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { spec: { entries: LoaderTarget[] }; params: LoaderParams; run_id?: string }) =>
      api<{ operation_id: string; state: string }>('/api/v1/loader/start', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['loader', 'active'] }),
  });
}

export function useLoaderLifecycle() {
  const qc = useQueryClient();
  const make = (verb: 'pause' | 'resume' | 'stop') =>
    useMutation({
      mutationFn: (id: string) => api(`/api/v1/loader/${id}/${verb}`, { method: 'POST' }),
      onSuccess: () => qc.invalidateQueries({ queryKey: ['loader'] }),
    });
  // We return three separate hooks-in-object so the caller can destructure.
  return {
    pause: make('pause'),
    resume: make('resume'),
    stop: make('stop'),
  };
}

export function useLoaderAdjust() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, params }: { id: string; params: LoaderParams }) =>
      api(`/api/v1/loader/${id}/params`, { method: 'PATCH', body: JSON.stringify(params) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['loader'] }),
  });
}

/* ---- deleter ---- */

export function useDeleterPreview() {
  return useMutation({
    mutationFn: (body: { spec: { entries: DeleteTarget[] }; params: { batch_size: number; max_ratio: number; inter_batch_jitter_ns?: number } }) =>
      api<DeletePreview>('/api/v1/deleter/preview', { method: 'POST', body: JSON.stringify(body) }),
  });
}

export function useDeleterStart() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { confirmation_token: string; run_id?: string }) =>
      api<{ operation_id: string; state: string }>('/api/v1/deleter/start', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['deleter'] }),
  });
}

export function useActiveDeleters() {
  return useQuery({
    queryKey: ['deleter', 'active'],
    queryFn: () => api<DeleterStatus[]>('/api/v1/deleter/active'),
    refetchInterval: 2_000,
  });
}

export function useDeleter(id: string | null) {
  return useQuery({
    queryKey: ['deleter', id],
    queryFn: () => api<DeleterStatus>(`/api/v1/deleter/${id}`),
    enabled: !!id,
    refetchInterval: 2_000,
  });
}

export function useDeleterLifecycle() {
  const qc = useQueryClient();
  const make = (verb: 'pause' | 'resume' | 'stop') =>
    useMutation({
      mutationFn: (id: string) => api(`/api/v1/deleter/${id}/${verb}`, { method: 'POST' }),
      onSuccess: () => qc.invalidateQueries({ queryKey: ['deleter'] }),
    });
  return { pause: make('pause'), resume: make('resume'), stop: make('stop') };
}

/* ---- compact ---- */

export function useCompactPreview() {
  return useMutation({
    mutationFn: (body: { scope: CompactScope; params: { max_replication_lag_ns: number; stepdown_wait_timeout_ns: number; validate_after_compact?: boolean } }) =>
      api<CompactPreview>('/api/v1/compact/preview', { method: 'POST', body: JSON.stringify(body) }),
  });
}

export function useCompactStart() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { scope: CompactScope; params: { max_replication_lag_ns: number; stepdown_wait_timeout_ns: number; validate_after_compact?: boolean }; run_id?: string }) =>
      api<{ operation_id: string; state: string }>('/api/v1/compact/start', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['compact'] }),
  });
}

export function useActiveCompacts() {
  return useQuery({
    queryKey: ['compact', 'active'],
    queryFn: () => api<CompactStatus[]>('/api/v1/compact/active'),
    refetchInterval: 2_000,
  });
}

export function useCompactCancel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api(`/api/v1/compact/${id}/cancel`, { method: 'POST' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['compact'] }),
  });
}

/* ---- workload ---- */

export function useWorkloadStart() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { spec: WorkloadSpec; params: WorkloadParams; run_id?: string }) =>
      api<{ operation_id: string; state: string }>('/api/v1/workload/start', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['workload'] }),
  });
}

export function useActiveWorkloads() {
  return useQuery({
    queryKey: ['workload', 'active'],
    queryFn: () => api<WorkloadStatus[]>('/api/v1/workload/active'),
    refetchInterval: 2_000,
  });
}

export function useWorkloadLifecycle() {
  const qc = useQueryClient();
  return {
    stop: useMutation({
      mutationFn: (id: string) => api(`/api/v1/workload/${id}/stop`, { method: 'POST' }),
      onSuccess: () => qc.invalidateQueries({ queryKey: ['workload'] }),
    }),
    setRate: useMutation({
      mutationFn: ({ id, params }: { id: string; params: WorkloadParams }) =>
        api(`/api/v1/workload/${id}/rate`, { method: 'PATCH', body: JSON.stringify(params) }),
      onSuccess: () => qc.invalidateQueries({ queryKey: ['workload'] }),
    }),
  };
}

/* ---- snapshots ---- */

export function useSnapshots(label?: string) {
  return useQuery({
    queryKey: ['snapshots', label ?? 'all'],
    queryFn: () => api<Snapshot[]>(`/api/v1/snapshots/${label ? `?label=${encodeURIComponent(label)}` : ''}`),
    refetchInterval: 15_000,
  });
}

export function useTakeSnapshot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { label: string; note?: string; run_id?: string }) =>
      api<{ id: string }>('/api/v1/snapshots/', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['snapshots'] }),
  });
}

export function useCompareSnapshots(a: string | null, b: string | null) {
  return useQuery({
    queryKey: ['snapshots', 'compare', a, b],
    queryFn: () => api<{
      a: { id: string; label: string; taken_at: string };
      b: { id: string; label: string; taken_at: string };
      diff: {
        databases: { name: string; storage_delta: number; data_delta: number; index_delta: number; storage_before: number; storage_after: number }[];
        collections: { database: string; collection: string; storage_delta: number; free_storage_delta: number; count_delta: number; storage_before: number; storage_after: number; fragmentation_before: number; fragmentation_after: number }[];
      };
    }>(`/api/v1/snapshots/compare?a=${encodeURIComponent(a!)}&b=${encodeURIComponent(b!)}`),
    enabled: !!a && !!b,
  });
}

/* ---- runs ---- */

export function useRuns() {
  return useQuery({
    queryKey: ['runs'],
    queryFn: () => api<Run[]>('/api/v1/runs/'),
    refetchInterval: 10_000,
  });
}

export function useRun(id: string | null) {
  return useQuery({
    queryKey: ['run', id],
    queryFn: () => api<Run>(`/api/v1/runs/${id}`),
    enabled: !!id,
  });
}

export function useCreateRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name?: string; config?: unknown; notes?: string }) =>
      api<Run>('/api/v1/runs/', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['runs'] }),
  });
}

export function useRunMetrics(id: string | null, params: { metric?: string; scope?: string; scope_id?: string; since?: string; until?: string; limit?: number } = {}) {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined) qs.set(k, String(v));
  return useQuery({
    queryKey: ['run', 'metrics', id, qs.toString()],
    queryFn: () => api<MetricSample[]>(`/api/v1/runs/${id}/metrics?${qs.toString()}`),
    enabled: !!id,
    refetchInterval: 5_000,
  });
}

/* ---- events ---- */

export function useEvents(category?: string, runID?: string) {
  const qs = new URLSearchParams();
  if (category) qs.set('category', category);
  if (runID) qs.set('run_id', runID);
  return useQuery({
    queryKey: ['events', qs.toString()],
    queryFn: () => api<EventRow[]>(`/api/v1/events/?${qs.toString()}`),
    refetchInterval: 5_000,
  });
}
