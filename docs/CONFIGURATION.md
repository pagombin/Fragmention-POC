# Configuration

`mfpoc` reads configuration from a YAML file (`--config path.yaml`) and
merges environment-variable overrides prefixed with `MFPOC_`. Nested keys
use underscores: `MFPOC_LOGGING_LEVEL=debug`.

See [`configs/config.example.yaml`](../configs/config.example.yaml) for a
fully-annotated template.

## Secret references

Strings matching `${env:VAR}` or `${file:/path}` are resolved at startup:

- `${env:MONGO_PASSWORD}` — reads the environment variable.
- `${file:/var/run/secrets/mongo/password}` — reads the trimmed contents.

Both forms also work for `auth.bearer_token`, `auth.basic_pass`, and
(unpacked via the URI) `mongo.uri`.

## Minimum viable config

```yaml
mongo:
  uri: "mongodb://localhost:27017/?replicaSet=rs0"
auth:
  enabled: true
  bearer_token: "${env:MFPOC_BEARER_TOKEN}"
```

## Full option matrix

The example file is the canonical source. This page will be filled with
per-option tables in Phase 19.
