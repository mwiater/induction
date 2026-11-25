# AIF v1 scoring contract

Primary correctness is deterministic verification against the private bank.
No model, consensus, or second language model is used as a correctness judge.

- `anchor_accuracy`: correct completed responses divided by graded anchors.
- `domain_accuracy`: correct graded responses by domain, with `n` shown.
- `adaptive_accuracy`: shown by difficulty; it is not pooled into a standardized score.
- `metamorphic`: original correctness, variant correctness, relation consistency,
  and both-correct are separate measures.
- `fault_detection`: sensitivity on invalid proposals and specificity on valid
  controls; balanced accuracy is shown only when both classes exist.
- `Brier`: `(confidence - correctness)^2`, only for valid probabilities.
- Calibration bins are `[0,.2)`, `[.2,.4)`, `[.4,.6)`, `[.6,.8)`, and `[.8,1]`.
- Wilson 95% intervals are used for simple binomial accuracy estimates.
- Transport, timeout, and unrun items are availability failures, not incorrect
  completed responses. Invalid completed JSON is a format failure and is not
  silently removed from completeness reporting.

Small sample sizes, correlated variants, adaptive selection, and synthetic
question families make close rankings unresolved. No absolute IQ or general
intelligence claim is permitted.
