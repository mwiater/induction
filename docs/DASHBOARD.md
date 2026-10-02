# Dashboard metrics

Use the dashboard generator when you want a server-free HTML view of telemetry
and responses saved by previous sessions. It reads persisted session data and
does not perform inference.

Generate a server-free metrics projection from saved and unsaved sessions:

```bash
induction dashboard generate
```

The source is `.sessions/`. Outputs are
`data/dashboard/session_metrics.json` and the self-contained
`data/dashboard/dashboard.html`. Raw transcripts, reasoning, responses,
properties, metrics, and slot payloads remain in the session files.

See the [CLI reference](CLI-REFERENCE.md#server-and-runtime-json-output) for
related command options.
