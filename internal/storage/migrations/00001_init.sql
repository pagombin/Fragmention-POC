-- +goose Up
-- +goose StatementBegin
CREATE TABLE runs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    config_json TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    cancelled_at TEXT,
    notes TEXT
);
CREATE INDEX idx_runs_status ON runs(status);
CREATE INDEX idx_runs_created_at ON runs(created_at);

CREATE TABLE snapshots (
    id TEXT PRIMARY KEY,
    run_id TEXT,
    label TEXT NOT NULL,
    taken_at TEXT NOT NULL,
    scope TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    raw_stats_json TEXT NOT NULL,
    note TEXT,
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL
);
CREATE INDEX idx_snapshots_run ON snapshots(run_id);
CREATE INDEX idx_snapshots_taken_at ON snapshots(taken_at);
CREATE INDEX idx_snapshots_label ON snapshots(label);

CREATE TABLE metrics_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT,
    timestamp TEXT NOT NULL,
    scope TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_value REAL NOT NULL,
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL
);
CREATE INDEX idx_ms_run_metric_ts ON metrics_samples(run_id, metric_name, timestamp);
CREATE INDEX idx_ms_timestamp ON metrics_samples(timestamp);
CREATE INDEX idx_ms_scope ON metrics_samples(scope, scope_id, metric_name, timestamp);

CREATE TABLE operations (
    id TEXT PRIMARY KEY,
    run_id TEXT,
    kind TEXT NOT NULL,
    target_json TEXT NOT NULL,
    params_json TEXT NOT NULL,
    state TEXT NOT NULL,
    started_at TEXT,
    paused_at TEXT,
    resumed_at TEXT,
    completed_at TEXT,
    stats_json TEXT,
    error_message TEXT,
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL
);
CREATE INDEX idx_ops_state ON operations(state);
CREATE INDEX idx_ops_kind ON operations(kind);
CREATE INDEX idx_ops_run ON operations(run_id);

CREATE TABLE operation_progress (
    operation_id TEXT NOT NULL,
    collection_key TEXT NOT NULL,
    total_target INTEGER NOT NULL DEFAULT 0,
    completed_count INTEGER NOT NULL DEFAULT 0,
    bytes_processed INTEGER NOT NULL DEFAULT 0,
    last_updated TEXT NOT NULL,
    PRIMARY KEY (operation_id, collection_key),
    FOREIGN KEY (operation_id) REFERENCES operations(id) ON DELETE CASCADE
);

CREATE TABLE delete_candidates (
    operation_id TEXT NOT NULL,
    collection_key TEXT NOT NULL,
    candidate_ids_blob BLOB NOT NULL,
    PRIMARY KEY (operation_id, collection_key),
    FOREIGN KEY (operation_id) REFERENCES operations(id) ON DELETE CASCADE
);

CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT,
    operation_id TEXT,
    timestamp TEXT NOT NULL,
    level TEXT NOT NULL,
    category TEXT NOT NULL,
    message TEXT NOT NULL,
    context_json TEXT,
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL,
    FOREIGN KEY (operation_id) REFERENCES operations(id) ON DELETE SET NULL
);
CREATE INDEX idx_events_timestamp ON events(timestamp);
CREATE INDEX idx_events_run ON events(run_id);
CREATE INDEX idx_events_op ON events(operation_id);
CREATE INDEX idx_events_category ON events(category);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS delete_candidates;
DROP TABLE IF EXISTS operation_progress;
DROP TABLE IF EXISTS operations;
DROP TABLE IF EXISTS metrics_samples;
DROP TABLE IF EXISTS snapshots;
DROP TABLE IF EXISTS runs;
-- +goose StatementEnd
