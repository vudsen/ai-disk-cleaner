# Scan performance baseline

Measured on 2026-10-19 before changing the gdu scan pipeline. This is a warm-cache, synthetic Linux baseline; it is not a cold-cache or real-user-data result.

## Environment

- Ubuntu 24.04.4 LTS, amd64; Go 1.26.2; gdu v5.34.0; `GOMAXPROCS=4`.
- Intel Core i5-6500, 4 cores / 4 threads; 16 GiB RAM. The benchmark directory was on `/dev/sda3`, ext4. The backing device type was not available in this environment.
- Fixture: 200 directories with 100 1 KiB files each, three nested directories, a hard link, a symlink, and an 8 MiB sparse file. Generated once per test process outside measured loops; the test process removes it on exit. Parser input was 1,409,915 bytes and produced 20,207 tree entries.
- The benchmark used Go's normal parallelism (`-4`). The fixture was repeatedly read in the same process; caches were warm. No cache drop, real-root scan, LLM request, or deletion was used.
- Commands: four benchmarks with `-benchtime=1s -count=5 -benchmem`; `UpdateStats` with `-benchtime=1x -count=5 -benchmem` because every operation needs a newly analyzed, unaggregated tree prepared outside the timer. The separate phase sample used `/usr/bin/time -v`.

## Five benchmark runs

Each row reports the five per-run `ns/op` values in milliseconds and their median. Allocation columns are the median reported by `-benchmem`.

| Benchmark | Five runs (ms/op) | Median | B/op | allocs/op |
| --- | --- | ---: | ---: | ---: |
| `BenchmarkGDUAnalyze` | 25.197, 25.269, 24.966, 24.433, 25.185 | 25.185 ms | 14,146,926 | 147,435 |
| `BenchmarkGDUUpdateStats` | 0.285, 0.274, 0.388, 0.288, 0.314 | 0.288 ms | 712 | 6 |
| `BenchmarkGDUEncodeJSON` | 10.413, 10.511, 10.451, 10.608, 10.603 | 10.511 ms | 7,646,366 | 181,277 |
| `BenchmarkGDUParseFileTree` | 83.133, 83.046, 83.205, 83.668, 83.390 | 83.205 ms | 27,287,086 | 1,109,782 |
| `BenchmarkGDUScanEndToEnd` | 121.598, 121.756, 125.324, 123.536, 122.685 | 122.685 ms | 49,115,988 | 1,438,538 |

For the `UpdateStats` row, each value is one timed operation on a fresh tree; tree preparation is excluded from the timer. The other four rows report the benchmark runner's one-second sample average for each of five runs.

## Same-process phase sample and peak RSS

One opt-in phase sample (`AI_DISK_PERF_SAMPLE=1`) logged 20,207 entries and these times on the same fixture: analyze 26.292 ms, update stats 0.310 ms, encode 14.785 ms, parse 83.176 ms. Their measured sum was 124.564 ms. In that pass, encode plus parse were 97.961 ms, about 78.6% of the staged phase sum. A separate immediate call to production `scanGDUContext` took 111.249 ms. The production time is reported separately; it is not inferred by adding the phase benchmark medians.

`/usr/bin/time -v` reported a maximum resident set size of 286,932 KiB (about 280.2 MiB) for the independent Go test process running the phase sample and production scan. This includes the Go runtime and test process; it is not a heap profile. No heap profile was collected.

## Decision

Encoding plus parsing is well above the project's 20% investigation threshold in the same-fixture phase sample, and parsing allocates about 27.3 MB per operation. A separate follow-up change to investigate direct tree conversion is justified. This baseline does not show that a replacement implementation or another scanning library will be faster. The current change keeps the gdu pipeline intact.
