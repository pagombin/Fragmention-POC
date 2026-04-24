// TypeScript mirrors of the Go response shapes. Keep in sync with
// internal/mongo/stats.go, internal/loader/params.go, etc.

export interface DatabaseSummary {
  name: string;
  collections: number;
  data_size: number;
  storage_size: number;
  index_size: number;
  fs_used_size: number;
  fs_total_size: number;
  sampled_at: string;
}

export interface CollectionSummary {
  database: string;
  name: string;
  type: 'regular' | 'capped' | 'timeseries' | 'view' | 'clustered' | 'system';
  count: number;
  size: number;
  storage_size: number;
  free_storage_size: number;
  total_index_size: number;
  num_indexes: number;
  avg_obj_size: number;
  compressor?: string;
  fragmentation_ratio: number;
  sampled_at: string;
}

export interface Topology {
  kind: 'standalone' | 'replica_set' | 'unknown';
  replica_set?: string;
  primary?: string;
  members: Member[];
  detected_at: string;
  sharded: boolean;
  max_replica_lag_ns: number;
}

export interface Member {
  id: number;
  name: string;
  state: string;
  health: number;
  self: boolean;
  optime: string;
  syncing_to?: string;
  lag_seconds: number;
}

export interface ServerInfo {
  Version: string;
  Major: number;
  Minor: number;
  Patch: number;
  FCV: string;
  StorageEngine: string;
  WiredTigerCodec: string;
  GitVersion: string;
}

export interface LoaderTarget {
  database: string;
  collection: string;
  bytes_target: number;
}

export interface LoaderParams {
  workers?: number;
  batch_size: number;
  docs_per_second?: number;
  write_concern?: string;
  seed?: number;
  load_run_id?: string;
  storage_headroom_percent?: number;
  force_start?: boolean;
}

export interface LoaderStatus {
  operation_id: string;
  state: OperationState;
  params: LoaderParams;
  stats: { docs_inserted: number; bytes_inserted: number; batches_ok: number; batches_failed: number; started_at: string };
  progress?: ProgressRow[];
}

export interface ProgressRow {
  collection_key: string;
  total_target: number;
  completed_count: number;
  bytes_processed: number;
  last_updated: string;
}

export type OperationState =
  | 'idle' | 'running' | 'paused' | 'stopping' | 'stopped'
  | 'completed' | 'failed' | 'interrupted';

export type DeletePattern = 'random_by_id' | 'range_by_field' | 'modulo' | 'ttl_simulated' | 'prefix_by_id';

export interface DeleteTarget {
  database: string;
  collection: string;
  pattern: {
    kind: DeletePattern;
    ratio?: number;
    field?: string;
    range_before?: string;
    modulus?: number;
    prefix?: string;
    seed?: number;
  };
}

export interface DeletePreview {
  confirmation_token: string;
  expires_at: string;
  per_collection: { database: string; collection: string; matched_count: number; total_documents: number }[];
  total_matches: number;
  requires_typed_confirmation: boolean;
  max_ratio_breached: boolean;
}

export interface DeleterStatus {
  operation_id: string;
  state: OperationState;
  params: { batch_size: number; inter_batch_jitter_ns: number; max_ratio: number };
  stats: { deleted: number; batches_ok: number; batches_failed: number; per_collection: Record<string, number> };
  progress?: ProgressRow[];
}

export interface CompactScope {
  kind: 'cluster' | 'databases' | 'collections';
  databases?: string[];
  collections?: { database: string; collection: string }[];
}

export interface CompactPreview {
  mode: 'single' | 'rolling';
  total_collections: number;
  estimated_duration_ns: number;
  warnings?: string[];
  execution_order: {
    member: string;
    role: string;
    collections: string[];
    requires_stepdown?: boolean;
  }[];
}

export interface CompactStatus {
  operation_id: string;
  state: OperationState;
  current_step?: string;
}

export interface WorkloadSpec {
  targets: { database: string; collection: string }[];
  read_weight: number;
  write_weight: number;
  aggregate_weight: number;
}

export interface WorkloadParams {
  target_ops_per_sec: number;
  workers: number;
}

export interface WorkloadStatus {
  operation_id: string;
  state: OperationState;
  params: WorkloadParams;
  ops_done: number;
  errors: number;
}

export interface Snapshot {
  id: string;
  run_id?: string | null;
  label: string;
  taken_at: string;
  scope: string;
  scope_id: string;
  raw_stats: unknown;
  note?: string;
}

export interface Run {
  id: string;
  name: string;
  config: unknown;
  status: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  cancelled_at?: string;
  notes?: string;
}

export interface EventRow {
  id: number;
  run_id?: string | null;
  operation_id?: string | null;
  timestamp: string;
  level: 'info' | 'warn' | 'error' | 'audit';
  category: string;
  message: string;
  context?: Record<string, unknown>;
}

export interface MetricSample {
  run_id?: string | null;
  timestamp: string;
  scope: string;
  scope_id: string;
  metric_name: string;
  value: number;
}
