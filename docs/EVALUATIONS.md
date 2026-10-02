# Evaluations

Use evaluations to run a configured local-model suite, compare one model or
all available models, and inspect normalized results. Evaluation commands are
beta features and require `--beta`.

Evaluations use the configuration passed to `--eval-config` and write
normalized results under `data/evals/results/`:

```bash
induction eval --beta --eval-config inspect_evals.example.yaml --model MODEL
induction eval status --beta --eval-config inspect_evals.example.yaml --model MODEL
induction eval --beta --eval-config inspect_evals.example.yaml --allModels
```

External Inspect AI dependencies must be installed separately. The checked-in
`inspect_evals.example.yaml` is a starting point for local evaluation suites.

See the [CLI reference](CLI-REFERENCE.md#evaluation-induction-eval) for the
available evaluation flags.
