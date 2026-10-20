## ADDED Requirements

### Requirement: Preserve the production scanner while measuring

This change SHALL retain gdu v5.34.0 and the existing JSON-based production pipeline. Measurement code MUST NOT introduce a new frontend bridge, scan engine or production optimization toggle.

#### Scenario: Complete this change
- **WHEN** Linux support and the performance report are delivered
- **THEN** production still uses AnalyzeDir, UpdateStats, JSON encoding and FileTree parsing with the added Linux policies

### Requirement: Reproducible stage measurements

Benchmarks SHALL separately measure AnalyzeDir, UpdateStats, complete in-memory JSON encoding, FileTree parsing including its aggregation, and end-to-end scanning. Setup MUST be outside timed sections and mutable stage inputs MUST NOT be reused incorrectly.

#### Scenario: Benchmark runs five times
- **WHEN** the documented benchmark command runs with benchmem and count=5
- **THEN** the report includes stage timing, B/op, allocs/op, fixture entry count and JSON byte size with environment and warm-cache conditions

#### Scenario: End-to-end memory is reported
- **WHEN** the report describes process peak memory
- **THEN** it distinguishes maximum RSS from cumulative allocations and Go heap profiles

### Requirement: Controlled benchmark datasets

Default datasets SHALL be generated in temporary directories with bounded storage and exclude setup from timings. An optional real dataset MUST be explicitly supplied and read-only. Benchmarks MUST NOT invoke deletion, migrations, LLM requests or system-wide cache flushing.

#### Scenario: No real dataset is configured
- **WHEN** AI_DISK_BENCH_ROOT is absent
- **THEN** controlled synthetic measurements run and the report explicitly states that real-data performance remains unverified

### Requirement: Evidence-based follow-up decision

The performance report SHALL apply design D9's decision threshold and name the measured bottleneck. Direct tree conversion or engine replacement MUST remain a separate change.

#### Scenario: JSON work is a substantial cost
- **WHEN** controlled measurements meet the JSON timing threshold or identify intermediate representations as a major peak-memory cost
- **THEN** the report recommends a separately scoped direct-conversion change with the supporting evidence

#### Scenario: Traversal dominates
- **WHEN** JSON work is below the timing threshold and memory evidence does not identify it as dominant
- **THEN** the report recommends investigating traversal or storage and makes no unsupported claim that another scanner is faster
