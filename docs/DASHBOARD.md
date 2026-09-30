# Dashboard metrics

Generate a server-free metrics projection from saved and unsaved sessions:

```bash
induction dashboard generate
```

The source is `.sessions/`. Outputs are
`data/dashboard/session_metrics.json` and the self-contained
`data/dashboard/dashboard.html`. Raw transcripts, reasoning, responses,
properties, metrics, and slot payloads remain in the session files.
