# Schema generation and compilation benchmark

Issue [#6](https://github.com/ronhuafeng/llmkit-go/issues/6) asks whether
`llmschema.Decode` should cache generated and compiled schemas. This document
records the benchmark-first result. The accompanying change intentionally does
not alter production code.

## Method

The benchmarks use five representative structured-output shapes: a scalar, a
small flat struct, a nested struct with slices, a nullable pointer-heavy
struct, and a struct containing a map and `json.RawMessage`. Each pipeline
benchmark measures these stages independently:

1. generate and marshal schema JSON with `SchemaJSONFor`;
2. decode, register, and compile that schema with the validator;
3. validate a pre-decoded instance with a precompiled schema;
4. unmarshal the typed result; and
5. run the existing end-to-end `Decode` path.

Additional benchmarks cover repeated decode of one type, a loop over seven
distinct types, parallel decode of one type, concurrent reuse of one compiled
validator, and an invalid-instance path. The benchmark file also probes
recursive schema support and characterizes schema-byte determinism and
`reflect.Type` key behavior.

Run the measurements with:

```sh
GOWORK=off go test -run '^$' -bench . -benchmem -benchtime=500ms -count=5 ./llmschema
GOWORK=off go test -race ./llmschema
```

## Results

The following are representative medians from an Apple M1 Pro (`darwin/arm64`,
Go 1.26.4). Absolute values will vary by machine; the stage proportions and
allocations are the decision inputs.

| Shape | Generate | Compile | Validate | Unmarshal | Decode | Generate + compile / Decode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Scalar | 2.70 us | 8.42 us | 0.15 us | 0.10 us | 12.93 us | 86% |
| Small flat | 14.22 us | 31.21 us | 0.69 us | 0.44 us | 49.41 us | 92% |
| Nested slices | 31.06 us | 66.51 us | 1.91 us | 1.20 us | 106.06 us | 92% |
| Nullable pointer-heavy | 31.71 us | 80.84 us | 1.71 us | 1.45 us | 122.28 us | 92% |
| Map and RawMessage | 18.81 us | 36.75 us | 1.21 us | 1.07 us | 62.17 us | 89% |

Generation plus compilation also accounts for roughly 85%--93% of end-to-end
allocations in these shapes. Representative whole-path results were:

| Path | Time | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: |
| Repeated nested type | 106.93 us | 97,276 | 1,225 |
| Seven distinct types per iteration | 392.95 us | 368,701 | 4,533 |
| Parallel nested type | 73.78 us | 97,891 | 1,229 |
| Invalid small struct | 50.10 us | 48,562 | 595 |
| Parallel precompiled validation | 1.11 us | 2,172 | 43 |

The parallel precompiled benchmark and `go test -race ./llmschema` completed
without a race. This is evidence that the validator object supports the tested
read-only concurrent validation path; it is not a proof for all validator
features or future dependency versions.

## Cache questions

- `SchemaJSONFor` produced byte-identical output across 100 repetitions for
  every benchmark shape. Determinism remains a dependency behavior to protect
  with tests if a cache is later implemented.
- `reflect.Type` gives aliases the target type's key, while distinct named
  types, pointer types, and a named versus anonymous struct receive distinct
  keys. Structurally identical anonymous types in one Go program are identical
  types and therefore share a key.
- The current schema generator rejects the recursive value type probe with a
  cycle-detected error, so recursive types are skipped rather than benchmarked.
- Caching schema JSON and compiled validators together would remove the two
  dominant stages from repeated decode. Caching only one leaves a large share
  of the measured cost in place.
- This benchmark does not establish safe error caching. Errors may include
  dependency behavior that deserves a fresh evaluation, so a later design
  should start without caching construction errors unless it proves stable
  semantics.
- A package-level cache keyed by type has no natural bound. Programs that
  manufacture many runtime struct types could retain schemas and validator
  graphs for process lifetime. The repository has no evidence about real-world
  type cardinality or plugin lifecycle behavior.

## Conclusion

Caching is performance-justified for repeated typed decoding in these
synthetic and representative structured-output workloads: schema generation
and compilation consume 86%--92% of measured `Decode` latency and most of its
allocations. The evidence does **not** justify adding a production cache in
this PR because bounded-memory expectations, construction-error semantics,
plugin lifetimes, and the concurrency contract across dependency upgrades are
not yet resolved.

If pursued, caching must be a separate issue and PR. That work should preserve
schema bytes, violation ordering, and error types; publish immutable entries;
remain private and race-safe; document process-lifetime memory retention; and
add dependency-version concurrency coverage. No public configuration DSL or
provider policy is warranted by these results.
