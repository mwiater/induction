# Decision Routing Pipeline Examples

Examples:

- [Conditional review routing](#single-decision-conditional-review-routing)
- [Multi-level decision tree](#multi-level-decision-tree-with-synthesis)

## Single-decision conditional review routing

### 1. Overview

Classifies a software report and conditionally runs the matching specialist review. It is useful for triage and auditable branch selection.

### 2. Steps

- **route-report:** Selects security, performance, correctness, or other with candidate probabilities.
- **security-review:** Runs only for a sufficiently confident security selection and gives findings and mitigations.
- **performance-review:** Runs only for a sufficiently confident performance selection and gives findings and mitigations.
- **correctness-review:** Runs only for a sufficiently confident correctness selection and gives findings and mitigations.
- **general-review:** Runs for other and gives broad engineering-risk findings and mitigations.

### 3. Expected final output

The selected branch's concise review and mitigations, with decision probabilities recorded and non-selected branches skipped.

### 4. Example YAML

[Open pipelines/pipeline.decision-routing-01.yaml in the repository](../pipelines/pipeline.decision-routing-01.yaml)

### 5. YAML source

```yaml
name: decision-routing-01
config: induction.yaml

steps:
  - name: route-report
    model: JEV5K-v0.3-4B-Q8_0
    nomcp: true
    userPrompt: |
      Choose the single most important review focus for this software
      engineering report. Return only the option token.

      A = security
      B = performance
      C = correctness
      D = other

      Report: A service accepts user supplied filenames and reads each
      referenced file before validating the path against an allowed root.
    decision:
      candidates:
        A: security
        B: performance
        C: correctness
        D: other
      topLogprobs: 20

  - name: security-review
    model: Qwen-3.5-9B-MTP-General-Q8_0
    nomcp: true
    when:
      decision: route-report
      equals: security
      minConfidence: 0.55
    systemPrompt: You are a careful software security reviewer.
    userPrompt: |
      Review the report for security risks. Give concise findings and
      concrete mitigations. Report:
      A service accepts user supplied filenames and reads each referenced
      file before validating the path against an allowed root.

  - name: performance-review
    model: Qwen-3.5-9B-MTP-General-Q8_0
    nomcp: true
    when:
      decision: route-report
      equals: performance
      minConfidence: 0.55
    systemPrompt: You are a careful software performance reviewer.
    userPrompt: |
      Review the report for performance risks. Give concise findings and
      concrete mitigations. Report:
      A service accepts user supplied filenames and reads each referenced
      file before validating the path against an allowed root.

  - name: correctness-review
    model: Qwen-3.5-9B-MTP-General-Q8_0
    nomcp: true
    when:
      decision: route-report
      equals: correctness
      minConfidence: 0.55
    systemPrompt: You are a careful software correctness reviewer.
    userPrompt: |
      Review the report for correctness risks. Give concise findings and
      concrete mitigations. Report:
      A service accepts user supplied filenames and reads each referenced
      file before validating the path against an allowed root.

  - name: general-review
    model: Qwen-3.5-9B-MTP-General-Q8_0
    nomcp: true
    when:
      decision: route-report
      equals: other
    systemPrompt: You are a careful software engineering reviewer.
    userPrompt: |
      Review the report for its most important engineering risks. Give
      concise findings and concrete mitigations. Report:
      A service accepts user supplied filenames and reads each referenced
      file before validating the path against an allowed root.
```

## Multi-level decision tree with synthesis

### 1. Overview

Decomposes a food-safety question into four bounded decisions and renders the complete path. It is useful for transparent rule-based reasoning where uncertainty and alternatives must remain visible.

### 2. Steps

- **outage-duration:** Classifies outage length as over four hours, four hours or less, or unknown.
- **refrigerator-temperature:** Classifies the measured temperature relative to 40°F.
- **perishable-foods:** Classifies whether covered perishable food is present.
- **unsafe-temperature-duration:** Determines whether four hours above 40°F is established without inferring a temperature history.
- **food-safety-decision-tree:** Renders all candidates and probabilities, marks the path, and gives a cautious recommendation using supplied FDA guidance.

### 3. Expected final output

A four-level tree showing every candidate and normalized probability, selected values in order, and a practical recommendation that distinguishes facts from unknown duration and does not treat probabilities as medical certainty.

### 4. Example YAML

[Open pipelines/pipeline.decision-routing-02.yaml in the repository](../pipelines/pipeline.decision-routing-02.yaml)

### 5. YAML source

```yaml
name: decision-routing-02-food-safety-after-power-outage
config: induction.yaml

steps:
  - name: outage-duration
    model: JEV5K-v0.3-4B-Q8_0
    nomcp: true
    systemPrompt: |
      You are a Jev-style decision model. Apply only the stated criterion to
      the supplied facts. Choose the best matching option. Do not explain.
    userPrompt: |
      Problem: A family lost power during a storm. The outage lasted six
      hours. The refrigerator door stayed closed. An appliance thermometer
      read 46°F (8°C) when power returned. The refrigerator contains raw
      chicken, milk, and leftovers. No thermometer history shows when its
      temperature first rose above 40°F (4°C).

      Criterion: Did the power outage last more than four hours?
      Return only the option token:
      A = over_four_hours
      B = four_hours_or_less
      C = unknown
    decision:
      candidates:
        A: over_four_hours
        B: four_hours_or_less
        C: unknown
      topLogprobs: 20

  - name: refrigerator-temperature
    model: JEV5K-v0.3-4B-Q8_0
    nomcp: true
    systemPrompt: |
      You are a Jev-style decision model. Apply only the stated criterion to
      the supplied facts. Choose the best matching option. Do not explain.
    userPrompt: |
      Problem facts: The power outage lasted six hours. The refrigerator
      door stayed closed. When power returned, an appliance thermometer
      read 46°F (8°C). No thermometer history shows when its temperature
      first rose above 40°F (4°C).

      Criterion: Was the measured refrigerator temperature above 40°F
      (4°C) when power returned?
      Return only the option token:
      A = above_40F
      B = at_or_below_40F
      C = unknown
    decision:
      candidates:
        A: above_40F
        B: at_or_below_40F
        C: unknown
      topLogprobs: 20

  - name: perishable-foods
    model: JEV5K-v0.3-4B-Q8_0
    nomcp: true
    systemPrompt: |
      You are a Jev-style decision model. Apply only the stated criterion to
      the supplied facts. Choose the best matching option. Do not explain.
    userPrompt: |
      Problem facts: The refrigerator contains raw chicken, milk, and
      leftovers.

      Criterion: Does the refrigerator contain perishable food covered by
      power-outage food-safety guidance?
      Return only the option token:
      A = perishable_food_present
      B = no_perishable_food
      C = unknown
    decision:
      candidates:
        A: perishable_food_present
        B: no_perishable_food
        C: unknown
      topLogprobs: 20

  - name: unsafe-temperature-duration
    model: JEV5K-v0.3-4B-Q8_0
    nomcp: true
    systemPrompt: |
      You are a Jev-style decision model. Do not infer a temperature history
      from a single thermometer reading. Choose unknown when the stated facts
      do not establish the duration.
    userPrompt: |
      Problem facts: The outage lasted six hours. The refrigerator door
      stayed closed. The thermometer read 46°F (8°C) when power returned.
      There is no thermometer history showing when the temperature first
      rose above 40°F (4°C).

      Criterion: Do the facts establish that the food was above 40°F (4°C)
      for four hours or more?
      Return only the option token:
      A = four_hours_or_more_established
      B = less_than_four_hours_established
      C = duration_unknown
    decision:
      candidates:
        A: four_hours_or_more_established
        B: less_than_four_hours_established
        C: duration_unknown
      topLogprobs: 20

  - name: food-safety-decision-tree
    model: Qwen-3.5-9B-MTP-General-Q8_0
    nomcp: true
    systemPrompt: |
      You are a clear, cautious food-safety explainer. Use the supplied
      decision results and FDA guidance. Do not invent facts or treat model
      probabilities as medical certainty. Clearly distinguish the measured
      facts from unknowns.
    userPrompt: |
      Print a readable four-level decision tree for this household food-safety
      problem. At every level, show every candidate's semantic value and
      normalized probability from that level's result, then identify the
      selected value. Do not omit alternatives or report only the winning
      probability. Format each level as a tree node with all candidate branches;
      mark the selected branch with `→` and the other branches with `├` or `└`.
      After the four levels, print the path followed as an ordered sequence of
      selected values.

      Use these complete decision results:
      Level 1, outage duration:
      {{steps.outage-duration.output}}

      Level 2, refrigerator temperature:
      {{steps.refrigerator-temperature.output}}

      Level 3, perishable foods present:
      {{steps.perishable-foods.output}}

      Level 4, duration above 40°F:
      {{steps.unsafe-temperature-duration.output}}

      Then give a concise practical recommendation for the raw chicken, milk,
      and leftovers. FDA guidance says the refrigerator keeps food cold for
      about four hours if unopened; refrigerated perishable food that has
      been above 40°F for four hours or more should be discarded. If it is
      unknown how long food has been at or above 40°F, do not take a chance
      with it. Do not use smell or appearance to decide whether food is safe.
      Explain how the unknown temperature history affects this case. Do not
      imply that the probabilities are calibrated medical certainty.

      Guidance source:
      https://www.fda.gov/food/buy-store-serve-safe-food/food-and-water-safety-during-power-outages-and-floods
```
