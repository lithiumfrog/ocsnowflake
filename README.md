# ocsnowflake

Tiny, dependency-free Snowflake ID generator for Go.

This package implements Twitter-style Snowflake IDs with a modern epoch and a clean, minimal API. It’s concurrency-safe, fast, and provides helpers for parsing, formatting, and JSON encoding.

- 64-bit IDs: 42 bits time, 10 bits node, 12 bits sequence
- Up to 4,096 IDs per millisecond per node
- 1,024 nodes (IDs 0–1023)
- Default epoch: 2025-10-10 10:10:10 UTC

PkgGoDev: https://pkg.go.dev/github.com/lithiumfrog/ocsnowflake


## Features

- Drop-in Twitter Snowflake compatibility with a friendlier default epoch.
- Fully dependency-free (std lib only) and safe for concurrent goroutines.
- Allocation-free hot path: `Generate()` returns a value type, and helpers reuse buffers where possible.
- Built-in parsing, formatting, and JSON helpers for painless interop with services and storage engines.


## Table of contents

- [Features](#features)
- [Requirements](#requirements)
- [Install](#install)
- [Quick start](#quick-start)
- [API overview](#api-overview)
- [ID layout](#id-layout)
- [Epoch and time](#epoch-and-time)
- [Concurrency and guarantees](#concurrency-and-guarantees)
- [Performance notes](#performance-notes)
- [JSON and parsing examples](#json-and-parsing-examples)
- [Batch generation](#batch-generation)
- [Gotchas and notes](#gotchas-and-notes)
- [Development](#development)


## Requirements

- Go 1.21 or newer (module uses generics-era stdlib features and assumes monotonic clocks available in `time`).
- Works on Linux, macOS, and Windows without CGO.
- No runtime configuration files—just import and go.


## Install

```bash
go get github.com/lithiumfrog/ocsnowflake
```


## Quick start

```go
package main

import (
    "fmt"
    "time"

    sf "github.com/lithiumfrog/ocsnowflake"
)

func main() {
    // Choose a node ID in [0, 1023]. This should be unique per process/machine.
    node, err := sf.NewNode(1)
    if err != nil {
        panic(err)
    }

    id := node.Generate()

    fmt.Println("ID (uint64):", id.Uint64())
    fmt.Println("ID (string):", id.String())
    fmt.Println("Node:", id.Node())
    fmt.Println("Sequence:", id.Sequence())

    // Get the embedded timestamp (milliseconds since Unix epoch)
    fmt.Println("Epoch time (int64):", id.EpochTimeInt64())
    fmt.Println("Epoch time (string):", id.EpochTimeString())

    // Convert to time.Time if needed
    t := time.UnixMilli(id.EpochTimeInt64())
    fmt.Println("Time:", t.UTC())
}
```


## API overview

Package import:

```go
import sf "github.com/lithiumfrog/ocsnowflake"
```

- Node lifecycle
	- `func NewNode(node int64) (*sf.Node, error)`
		- Create a node for a given node ID (0–1023). Safe for concurrent use.
	- `func (n *Node) Generate() ID`
		- Generate a new Snowflake ID.
    - `func (n *Node) GenerateBatch(count int) []ID`
        - Generate multiple Snowflake IDs in a single call for better performance (count must be > 0).

- ID helpers
	- `type ID uint64`
	- `func (f ID) Uint64() uint64`
	- `func (f ID) String() string`
	- `func (f ID) Bytes() []byte`
	- `func (f ID) EpochTimeInt64() int64`  → milliseconds since Unix epoch
	- `func (f ID) EpochTimeString() string`  → milliseconds since Unix epoch
	- `func (f ID) Node() int64`
	- `func (f ID) Sequence() int64`
	- `func ParseUint64(u uint64) ID`
	- `func ParseString(s string) (ID, error)`
	- `func ParseBytes(b []byte) (ID, error)`

- JSON
	- `func (f ID) MarshalJSON() ([]byte, error)`
	- `func (f *ID) UnmarshalJSON(b []byte) error`
	- JSON is encoded as a quoted decimal string (safe for JavaScript and large integers).

- Database (`database/sql` / `driver`)
	- `func (f ID) Value() (driver.Value, error)`
	- `func (f *ID) Scan(src any) error`
	- Implements `driver.Valuer` and `sql.Scanner` so an ID round-trips through a Postgres `BIGINT` (signed `int64`) column via two's-complement reinterpretation—lossless across the full `uint64` range. `Scan` accepts `int64`, `uint64`, `[]byte`/`string` (decimal), and `nil` (→ 0).


## ID layout

```
 0                                                               63
 +----------------------------------------------------------------+
 | 42-bit time (ms since epoch) | 10-bit node | 12-bit sequence   |
 +----------------------------------------------------------------+
														 ^ timeShift=22     ^ nodeShift=12
```

- Time: milliseconds since the package epoch (42 bits)
- Node: node identifier (10 bits, 0–1023)
- Sequence: per-millisecond counter (12 bits, 0–4095)

When more than 4096 IDs are requested within the same millisecond on a single node, the generator blocks briefly until the next millisecond.


## Epoch and time

- Default epoch (exported var): `ocsnowflake.Epoch = 1760091010000` (ms) → 2025-10-10 10:10:10 UTC.
- `ID.EpochTimeInt64()` returns milliseconds since the Unix epoch as an `int64`.
- `ID.EpochTimeString()` returns milliseconds since the Unix epoch as a `string`.
- You can convert to `time.Time`:

```go
ts := time.UnixMilli(id.EpochTimeInt64())
```

If you need a different epoch, set it before creating any nodes:

```go
// Example: set epoch to 2020-01-01 00:00:00 UTC
ocsnowflake.Epoch = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
node, _ := ocsnowflake.NewNode(0)
```


## Concurrency and guarantees

- `Node` is safe for concurrent use by multiple goroutines.
- Uniqueness is guaranteed per-node: up to 4096 IDs per millisecond per node.
- To ensure global uniqueness, use a distinct node ID for each process/service instance.


## Performance notes

This package is optimized for single-threaded use, making it ideal for edge servers and lightweight applications. The thread-safe API provides flexibility for any Go application, though maximum throughput (4096 IDs/ms) is achieved when a single goroutine generates IDs.

**Best practices for maximum throughput:**

- **Use batch generation**: Call `GenerateBatch(count)` when you need multiple IDs at once—it's significantly faster than calling `Generate()` in a loop:

```go
ids := node.GenerateBatch(1000)  // Generate 1000 IDs efficiently
```

- **Single-threaded usage**: Generate IDs directly where needed without additional goroutines:

```go
// In a web handler
func handleRequest(w http.ResponseWriter, r *http.Request) {
    ids := node.GenerateBatch(10)
    // Process request with IDs...
}

// In a data processing pipeline
func processRecords(records []Record) {
    for _, rec := range records {
        rec.ID = node.Generate()
        // Process record...
    }
}
```

- **Minimize allocations**: Use `ID.Uint64()` directly when possible instead of converting to strings or bytes repeatedly.
- **Avoid I/O in hot paths**: Don't log, write to disk, or make network calls between ID generations if you need maximum throughput.

**Multi-threaded usage**: While `Node` is thread-safe and can be used concurrently from multiple goroutines, this creates lock contention and reduces throughput. For concurrent workloads, consider:
- Dedicating one goroutine per node with a buffered channel for distribution
- Partitioning work across multiple nodes with different node IDs (e.g., node 0 for worker 0, node 1 for worker 1)
- Using a single shared node if throughput requirements are modest (hundreds to low thousands of IDs/sec)

## Benchmarks

Benchmark helpers live alongside the tests; run them with:

```bash
go test -bench=. ./...
```

Sample run on Linux (Intel Xeon W-10855M @ 2.80GHz):

| Benchmark | Time/op | Bytes/op | Allocs/op | IDs/ms* |
|-----------|---------|----------|-----------|---------|
| Generate | 245 ns | 8 | 1 | ≈4.1k |
| GenerateBatch (n=16) | 3.90 µs | 128 | 1 | ≈4.1k |
| GenerateBatch (n=64) | 15.6 µs | 512 | 1 | ≈4.1k |
| GenerateBatch (n=256) | 62.6 µs | 2,048 | 1 | ≈4.1k |
| GenerateBatch (n=1024) | 250 µs | 8,192 | 1 | ≈4.1k |
| GenerateUncapped | 32 ns | 8 | 1 | ≈31k |
| GenerateBatchUncapped (n=16) | 95 ns | 128 | 1 | ≈169k |
| GenerateBatchUncapped (n=64) | 0.29 µs | 512 | 1 | ≈224k |
| GenerateBatchUncapped (n=256) | 1.20 µs | 2,048 | 1 | ≈213k |
| GenerateBatchUncapped (n=1024) | 4.21 µs | 8,192 | 1 | ≈243k |

\*IDs/ms = batch size ÷ (time/op in milliseconds). Values round to the theoretical 4,096 IDs/ms per node.

Numbers will vary based on CPU, Go version, and power settings, but they highlight the allocation-free hot path and linear scaling of batch generation. The uncapped benchmarks reuse the same allocation and locking behavior as production nodes while bypassing the per-millisecond wait, providing a ceiling for what your CPU can handle without the 4096 IDs/ms safety limit.


## JSON and parsing examples

```go
// JSON marshal (quoted decimal string)
b, _ := json.Marshal(id)

// JSON unmarshal
var id2 sf.ID
_ = json.Unmarshal(b, &id2)

// Parse from string/bytes
id3, _ := sf.ParseString("123456789012345678")
id4, _ := sf.ParseBytes([]byte("123456789012345678"))

// From uint64
id5 := sf.ParseUint64(42)
```

An `ID` can be written to and read from a SQL `BIGINT` column directly (works
with `database/sql` and pgx):

```go
// Write: ID is stored as the column's signed int64.
_, _ = db.Exec("INSERT INTO things (id) VALUES ($1)", id)

// Read: scans the int64 back into an ID losslessly.
var got sf.ID
_ = db.QueryRow("SELECT id FROM things WHERE id = $1", id).Scan(&got)
```


## Batch generation

When you need to generate multiple IDs, `GenerateBatch()` is significantly faster than calling `Generate()` repeatedly:

```go
// Generate 1000 IDs in one call
ids := node.GenerateBatch(1000)

// Process them as needed
for _, id := range ids {
    fmt.Println(id.String())
}
```

**Performance characteristics:**

- **Single lock acquisition**: `GenerateBatch()` acquires the internal lock once for the entire batch, while `Generate()` called N times acquires it N times.
- **No allocations per ID**: The batch is pre-allocated to the requested size.
- **Optimal for bulk operations**: Use when inserting many records, generating ID pools, or pre-allocating IDs for a request batch.

**When to use batch generation:**

- Inserting multiple database records in a transaction
- Pre-generating IDs for a batch API request
- Processing multiple items in a loop or pipeline
- Any scenario where you know you need multiple IDs upfront

**Batch size considerations:**

- Batches exceeding 4096 may span multiple milliseconds (the generator will wait as needed)



## Gotchas and notes

- Node IDs must be between 0 and 1023 (inclusive). Choosing unique node IDs is your responsibility.
- `ID.EpochTimeInt64()` returns milliseconds since the Unix epoch. Use `time.UnixMilli()` for conversion to `time.Time`.
- If system time moves backwards across the custom epoch boundary, the generator clamps to zero time offset; if sequence overflows within a millisecond, it waits for the next millisecond.
- The package uses a monotonic time offset where available to avoid issues with wall clock changes.


## Development

Use the standard Go toolchain to lint, test, and benchmark the package:

```bash
go test ./...
go test -bench=. ./...
```

Benchmarks will exercise the single-node hot path; run them on a quiet machine for stable numbers.

## License

This project is licensed under the [MIT License](./LICENSE).
