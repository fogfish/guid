<p align="center">
  <h3 align="center">GUID</h3>
  <p align="center"><strong>K-ordered unique identifiers in lock-free and
decentralized manner for Golang applications</strong></p>

  <p align="center">
    <!-- Version -->
    <a href="https://github.com/fogfish/guid/releases">
      <img src="https://img.shields.io/github/v/tag/fogfish/guid?label=version" />
    </a>
    <!-- Documentation -->
    <a href="http://godoc.org/github.com/fogfish/guid">
      <img src="https://godoc.org/github.com/fogfish/guid?status.svg" />
    </a>
    <!-- Build Status  -->
    <a href="https://github.com/fogfish/guid/actions/">
      <img src="https://github.com/fogfish/guid/workflows/test/badge.svg?branch=main" />
    </a>
    <!-- GitHub -->
    <a href="http://github.com/fogfish/guid">
      <img src="https://img.shields.io/github/last-commit/fogfish/guid.svg" />
    </a>
    <!-- Coverage -->
    <a href="https://coveralls.io/github/fogfish/guid?branch=main">
      <img src="https://coveralls.io/repos/github/fogfish/guid/badge.svg?branch=main" />
    </a>
    <!-- Go Card -->
    <a href="https://goreportcard.com/report/github.com/fogfish/guid">
      <img src="https://goreportcard.com/badge/github.com/fogfish/guid" />
    </a>
  </p>
</p>

---

Package guid implements interface to generate k-ordered unique identifiers in lock-free and decentralized manner for Golang applications. We says that sequence A is k-ordered if it consists of strictly ordered subsequences of length k:

```
  𝑨[𝒊 − 𝒌] ≤ 𝑨[𝒊] ≤ 𝑨[𝒊 + 𝒌] for all 𝒊 such that 𝒌 < 𝒊 ≤ 𝒏−𝒌.
```

## Key features

This library aims important objectives:

* **The allocator's location is part of the sort order.** ⟨𝒍⟩ outranks the fine fraction of time, so every node's identifiers occupy a contiguous, individually ordered run of the key space. The topology is readable from the keys themselves.
* IDs allocation does not require centralized authority, coordination between nodes, or a synchronized clock.
* IDs are suitable for partial event ordering in distributed environment and helps on detection of causality violation.
* IDs are roughly sortable by allocation order ("time").
* IDs reduce indexes footprints and optimize lookup latency.


## Inspiration

The event ordering in distributed computing is resolved using various techniques, e.g. Lamport timestamps, Universal Unique Identifiers, Twitter Snowflake and many other techniques are offered by open source libraries. `guid` is a Golang port of [Erlang's uid library](https://github.com/fogfish/uid).

All these solution made a common conclusion, globally unique ID is a triple ⟨𝒕, 𝒍, 𝒔⟩: ⟨𝒕⟩ monotonically increasing clock or timestamp is a primary dimension to roughly sort events, ⟨𝒍⟩ is spatially unique identifier of ID allocator so called node location, ⟨𝒔⟩ sequence is a monotonic integer, which prevents clock collisions. The `guid` library addresses few issues observed in other solutions.

Every byte counts when application is processing or storing large volume of events. This library implements fixed size 96-bit identity schema, which is castable to 64-bit under certain occasion. It is about 25% improvement to compare with UUID or similar 128-bit identity schemas (only Twitters Snowflake is 64-bit). The same schema is also offered at 128 bits as an RFC 9562 UUIDv8, `guid.X`, for deployments where interoperability outweighs the footprint.

Most of identity schemas uses monotonically increasing clock (timestamp) to roughly order events. The resolution of clock varies from nanoseconds to milliseconds. We found that usage of timestamp is not perfectly aligned with the goal of decentralized ID allocations. Usage of time synchronization protocol becomes necessary at distributed systems. Strictly speaking, NTP server becomes an authority to coordinate clock synchronization. This happens because schemas uses time fraction ⟨𝒕⟩ as a primary sorting key. In contrast with other libraries, `guid` do not give priority to single fraction of identity triple ⟨𝒕⟩ or ⟨𝒍⟩. It uses dynamic schema where the location fraction has higher priority than time only at particular precision.

**This is what the library exists for.** Inside one drift window the key space is partitioned by allocator: every node's identifiers form a contiguous run, and each run is exactly ordered — never interleaved with another node's. Consider a ring topology with leader-follower hand-over. A leader fails silently; for some interval two nodes believe they own the same range. Under a timestamp-primary schema their writes interleave, and recovering *who wrote what* means carrying the node identity somewhere else and filtering. Here the two nodes land in separate ranges: you scan a node's contribution directly, bound the overlap and reconcile, because the key space itself records the topology.

Neither Snowflake nor UUIDv7 can express this. Snowflake carries a machine id but places it below the *whole* timestamp, so grouping by node survives only within one millisecond. UUIDv7 has no location fraction at all — after the 48-bit millisecond it is entropy. The property is not a matter of tuning; the field has to be positioned to say it.

⟨𝒅⟩ is how wide that window is.

## Identity Schema

A fixed size of 96-bit is used to implement identity schema

```
  3bit  47 bit - 𝒅 bit         32 bit      𝒅 bit  14 bit
   |-|-------------------|----------------|-----|-------|
   ⟨𝒅⟩        ⟨𝒕⟩                ⟨𝒍⟩         ⟨𝒕⟩     ⟨𝒔⟩
```

↣ ⟨𝒕⟩ is 47-bit UTC timestamp with millisecond precision. It is derived from nanosecond UNIX timestamp by shifting it by 17 bits (time.Now().UnixNano() << 17). The library is able to change the base timestamp to any value in-order to address Year 2038 problem.

↣ ⟨𝒍⟩ is 32-bits node/allocator identifier. It is allocated randomly to each node using cryptographic random generator or application provided value. The node identity has higher sorting priority than the low bits of the timestamp, which is what makes each allocator's output a contiguous, exactly ordered run of the key space. The random allocation give an application ability to introduce about 65K allocators before it meets a high probability of collisions.

> If ⟨𝒍⟩ is meant to carry topology, assign it rather than randomize it. `guid.WithNodeRandom` — the default — gives distinct allocators, but random identities sort arbitrarily, so adjacent ring positions land far apart. Derive ⟨𝒍⟩ from the ring position with `guid.WithNodeID(...)` when you want the sort order to follow the topology.

↣ ⟨𝒅⟩ is 3 drift bits defines the width Δ of the window inside which ⟨𝒍⟩ outranks time. It shows the value of less important faction of time. The code selects a rung of an eight step ladder that runs from 1.05 ms to 73 minutes, configured on the clock with `guid.WithDrift(...)` and defaulting to about 4.5 minutes.

**Read Δ as a failover budget, not as a clock-skew budget.** It has to cover the interval between a silent failure and the moment the cluster has converged on a new owner — because that is the interval during which two allocators write to the same range and you need their output kept apart:

| constant | Δ | covers |
|---|---|---|
| `guid.Drift1ms` | 1.05 ms | ordering first — the class Snowflake and UUIDv7 occupy, for clocks that are actually synchronized |
| `guid.Drift16ms` | 16.8 ms | one datacenter, disciplined NTP |
| `guid.Drift268ms` | 268 ms | multiple regions synchronized over a WAN |
| `guid.Drift2s` | 2.15 s | consumer devices with working time sync |
| `guid.Drift17s` | 17.2 s | lease expiry, fast failure detectors |
| `guid.Drift275s` *(default)* | 274.9 s | gossip / phi-accrual failure detection, unmanaged clocks |
| `guid.Drift1099s` | 1099 s | slow membership convergence, cross-region hand-over |
| `guid.Drift4398s` | 4398 s | human-in-the-loop failover |

The four sub-second rungs are the interesting new range: at Δ = 1.05 ms the schema is in the same ordering class as Snowflake and UUIDv7, and ⟨𝒅⟩ becomes a dial between *timestamp-primary* and *location-primary* rather than a fixed opinion. Note what the low rungs really cost, though — the window is Δ + 2ε, so below a second or so it is the quality of your clocks, not the setting, that decides the ordering.

The same number is also the clock disagreement the ordering tolerates, which is why one knob serves both: two nodes whose clocks differ by less than Δ still sort into the same window. That matters because the target is not a managed cluster with datacenter NTP. On uncoordinated nodes — hardware without an RTC, devices behind firewalls that block NTP, VMs resuming from a snapshot, phones returning from airplane mode — tens of seconds of disagreement is the distribution, not a pathology. A timestamp-primary schema answers this by making an NTP server the coordinating authority, which is the coordination the library set out to avoid.

The drift must be the same for every value of a keyspace — ⟨𝒅⟩ is the most significant faction, so values allocated with different drift are segregated rather than interleaved. This is why it is a property of the clock and not an argument of `NewG` / `NewL`.

```go
clock := guid.NewClock(guid.WithDrift(guid.Drift16ms))

// guid.DriftOf picks the smallest rung that covers a tolerance
clock := guid.NewClock(guid.WithDrift(guid.DriftOf(60 * time.Second)))
```

↣ ⟨𝒔⟩ is 14-bit of monotonic strictly locally ordered integer. It helps to avoid collisions when multiple events happens during single millisecond or when the clock set backwards. The 14-bit value allows to have about 16K allocations per tick of ⟨𝒕⟩ and over 100M per second on single node. Each instance of application process runs a unique sequence of integers. The implementation ensures that the same integer is not returned more than once on the current
process. Restart of the process resets the sequence.

⟨𝒕⟩ and ⟨𝒔⟩ are allocated together, as a single atomic step, so that the pair ⟨𝒕,𝒔⟩ strictly increases with every allocation. ⟨𝒔⟩ counts within one tick of ⟨𝒕⟩ and restarts when the clock ticks; an allocator that exhausts a tick carries into the next one rather than folding ⟨𝒔⟩ back to zero. This makes values allocated by a single process ordered exactly as they were allocated — at any allocation rate, and even across a clock that is stepped backwards, where ⟨𝒕⟩ holds its high water mark until real time catches up. A process that saturates the allocator — a loop that allocates and discards, nothing else — makes ⟨𝒕⟩ run ahead of the wall clock; the ordering is unaffected and the gap closes on its own once the loop stops. [§3.6 of the proof](doc/proof.md) derives the rates, for readers who need `Time` to track real time under synthetic load.

The ordering guarantees are stated and proven in [doc/proof.md](doc/proof.md); statement (2) is machine-checked in [doc/proof.lean](doc/proof.lean).

The library supports casting of 96-bit identifier to 64-bit by dropping ⟨𝒍⟩ fraction. This optimization reduces a storage footprint if application uses persistent allocators.

```
  3bit        47 bit            14 bit
   |-|------------------------|-------|
   ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩
```

The same schema is also defined at 128 bits, where ⟨𝒍⟩ is 58 bits wide and the 6 bits RFC 9562 reserves for the version and the variant make the value a UUID, see `guid.X`.

```
  3bit  47 bit - 𝒅 bit             58 bit          𝒅 bit  14 bit
   |-|-------------------|--------------------------|-----|-------|
   ⟨𝒅⟩        ⟨𝒕⟩                    ⟨𝒍⟩               ⟨𝒕⟩     ⟨𝒔⟩
```

## Types

The library defines three types, each occupying exactly the bits its schema needs, so that an application storing millions of identifiers pays for nothing it does not use.

| type | schema | size | ⟨𝒍⟩ | alignment | allocator |
|---|---|---|---|---|---|
| `guid.X` | ⟨𝒅,𝒕,𝒍,𝒔⟩ | **16 bytes** | 58 bit | 1 | `guid.NewX(clock)` |
| `guid.G` | ⟨𝒅,𝒕,𝒍,𝒔⟩ | **12 bytes** | 32 bit | 1 | `guid.NewG(clock)` |
| `guid.L` | ⟨𝒅,𝒕,𝒔⟩   | **8 bytes**  | —      | 8 | `guid.NewL(clock)` |

All three carry the same fractions in the same order, and share the same drift ladder, clock and sequencer. They differ only in how much room is left for the node identity, which is also what separates their use cases.

`guid.G` holds the big-endian representation of the 96-bit number the schema defines, and nothing else. The representation is the one the schema is defined in, so memory I/O costs nothing:

* a `[]guid.G` packs without padding, 12 bytes per value;
* the wire format is the value itself — `g[:]` is a valid encoding and `copy(g[:], buf)` a valid decoding, neither shifts a single bit;
* the byte order is the order of the identifiers, so `bytes.Compare` over the raw bytes agrees with `g.Before`. An index that sorts the stored bytes — a B-tree, a sorted file, a key-value store — orders the identifiers correctly without decoding them.

`guid.L` is the 64-bit number itself, one machine word: passed in registers, compared with a single instruction, stored in 8 bytes.

`guid.X` is the same schema at 128 bits, and it spends the extra 4 bytes on standards compliance rather than on the clock. Six of the 128 bits are the version and variant fields [RFC 9562](https://www.rfc-editor.org/rfc/rfc9562) fixes, which leaves 122 for the schema and widens ⟨𝒍⟩ to 58 bits — so an `X` **is** a UUID of version 8, not "UUID-shaped":

* it drops into every `uuid` column in PostgreSQL, MySQL and SQL Server, every UUID library in every language, every debugger and log viewer — rendering correctly rather than as a malformed v7;
* `x.String()` emits the canonical `xxxxxxxx-xxxx-8xxx-yxxx-xxxxxxxxxxxx` form rather than the private alphabet `G` and `L` use, because the whole point is that other systems recognise it. `x.Base62()` remains available as the compact representation;
* it is readable by non-Go systems without porting anything — which matters for a schema whose premise is allocation across uncoordinated nodes, since a cluster of uncoordinated nodes is rarely a cluster of uniform Go processes.

The reserved bits cost 6 bits of payload and nothing else. They are *constants of the format*, so at each of those positions two values are identical and a most-significant-first comparison falls through to the next one: lexicographic order over the 128 bits is exactly lexicographic order over the variable payload, in field order. `bytes.Compare` still agrees with `x.Before`.

The three are distinct types, which is deliberate — values of different width are not comparable, and the compiler now says so. Values of two different types **must never share a keyspace** either: they are different widths with different field semantics and no ordering relation between them is defined. Convert explicitly at the boundary.

### Decoding and casting

Only allocation is a package function. Everything that turns data you already hold into a value is a **method on the destination**, so the type you are building is the receiver and the compiler picks the decoder:

```go
var uid guid.X
err := uid.FromString("06377f2a-0cb8-8a3f-b000-0000000003e9")
```

This is the one place the schema's cardinal rule can be broken by a typo — a value of the wrong type decoded into a keyspace is unrecoverable — so it is not left to a suffix on a function name that the reader has to notice. There is no `FromStringG` to write instead of `FromStringX`; `g.FromString` reads a `G` because `g` is a `G`.

Every accessor has its inverse on the same type:

| encode | decode |
|---|---|
| `uid.Bytes()` | `uid.FromBytes(b)` |
| `uid.String()` | `uid.FromString(s)` |
| `uid.Base62()` | `uid.FromBase62(s)` |
| `uid.Split(n)` | `uid.Fold(n, b)` |
| `uid.Epoch()` | `uid.FromTime(clock, t)` |

Casts read in the direction of the assignment:

```go
g.FromL(clock, l)   // 64-bit  -> 96-bit,  stamps the ⟨𝒍⟩ fraction
g.FromX(x)          // 128-bit -> 96-bit,  truncates ⟨𝒍⟩ to 32 bits
l.FromG(g)          // drops ⟨𝒍⟩
l.FromX(x)          // drops ⟨𝒍⟩
x.FromG(g)          // 96-bit  -> 128-bit, ⟨𝒍⟩ keeps the 32 bits it had
x.FromL(clock, l)   // 64-bit  -> 128-bit, stamps the ⟨𝒍⟩ fraction
```

A decoder that returns an error **leaves the destination unchanged**, so a value already there survives a failed decode rather than being half overwritten. The receiver has to be addressable — a variable, a struct field and a slice element are; a map element or a function result is not, and needs a temporary.

All three types implement `encoding.TextMarshaler`, `TextUnmarshaler`, `BinaryMarshaler` and `BinaryUnmarshaler`, so they travel through `gob`, yaml, toml or any other codec that speaks the stdlib interfaces without that codec knowing this package exists.

### Which one to use

**Reach for `guid.L` when you need a sortable key in a database.** It is 8 bytes — half of a UUID, a third smaller than `guid.G`, and the same width as a `BIGINT` — and it is *strictly* ordered rather than k-ordered: values are returned in exactly the order they were allocated, at any rate, and across a clock that NTP steps backwards. There is no drift window to reason about because there is no ⟨𝒍⟩ fraction for time to rank against. For surrogate keys, event ids and sequence numbers this is the better instrument, and the cheaper one.

Its limit is in the name: a local value is unique within the allocator that issued it. Use it where the surrounding context already disambiguates — a per-tenant or per-partition key space, a single writer, or a row that already carries the node — and use `guid.G` where it does not.

**Reach for `guid.G` when identifiers are allocated by many uncoordinated nodes** and you want the topology in the key: a contiguous, exactly ordered run per allocator, so that overlapping writers can be told apart and reconciled. That is what the extra 4 bytes buy. Its ⟨𝒍⟩ is 32 bits, so randomly allocated node identities meet a birthday bound at about 65 000 allocators.

**Reach for `guid.X` when the identifiers leave Go, or when there are more allocators than 32 bits of ⟨𝒍⟩ can keep apart.** Its 58-bit ⟨𝒍⟩ moves the birthday bound to about 5.4·10⁸, which takes node collision out of the set of things an operator has to think about — and uniqueness, not ordering, is this library's real exposure: the ordering is machine-checked, the node distinctness is assumed. Against UUIDv7, which it costs exactly as much to store, it adds strict ordering *within* a millisecond, topology in the key, 131 µs time resolution, exact intra-node sequencing and a proof. Against UUIDv7 it also asks for two things UUIDv7 never asks for — a drift held constant across the cluster and across the lifetime of the data, and node identities that stay distinct. Both are easy on day one and are the shape of a year-three incident; if a sortable primary key is all you need, UUIDv7's zero configuration and unconditional uniqueness are the better trade.

| situation | type |
|---|---|
| sortable key, one allocator or a disambiguating context | `guid.L` — 8 B |
| many uncoordinated allocators, storage footprint matters | `guid.G` — 12 B |
| many uncoordinated allocators, interop or node count matters | `guid.X` — 16 B, a UUID |

```go
x := guid.NewX(guid.Clock)   // 128-bit, globally unique, an RFC 9562 UUIDv8
g := guid.NewG(guid.Clock)   // 96-bit, globally unique
l := guid.NewL(guid.Clock)   // 64-bit, unique within this allocator

g.Time()  g.Node()  g.Seq()  g.Epoch()
g.Before(other)  g.After(other)  g.Equal(other)
g.String()  g.Base62()  g.Bytes()

x.String()                   // "0198c4f1-a35c-8b7e-b2a0-91d5e0c00001"
err := x.FromString(s)       // the decoder is a method on what it builds
```

## Migrating from v2

v3 is a compatibility break. The bit layout of the identifiers is unchanged — values encoded by v2 decode in v3 and sort the same way — but the Go types and the API around them are new.

| v2 | v3 |
|---|---|
| `guid.K` (one 128-bit struct for both shapes) | `guid.G` (12 bytes) and `guid.L` (8 bytes), distinct types |
| `guid.G(clock)` / `guid.L(clock)` | `guid.NewG(clock)` / `guid.NewL(clock)` |
| `guid.Z(clock)` | `guid.ZeroG(clock)` / `guid.ZeroL(clock)` |
| `guid.Time(uid)`, `guid.Node(uid)`, `guid.Seq(uid)` | `uid.Time()`, `uid.Node()`, `uid.Seq()` |
| `guid.Before(a, b)`, `guid.After`, `guid.Equal`, `guid.Diff` | `a.Before(b)`, `a.After(b)`, `a.Equal(b)`, `a.Diff(b)` |
| `guid.EpochT(uid)`, `guid.EpochI(uid)` | `uid.Epoch()` — one method, see below |
| `guid.String(uid)`, `guid.Bytes(uid)`, `guid.Base62(uid)` | `uid.String()`, `uid.Bytes()`, `uid.Base62()`, each with an inverse `uid.FromString(s)`, `uid.FromBytes(b)`, `uid.FromBase62(s)` |
| `guid.FromL(clock, uid)` / `guid.ToL(uid)` | `g.FromL(clock, l)` / `l.FromG(g)` |
| `guid.FromBytes(b)` (dispatched on length) | `g.FromBytes(b)` / `l.FromBytes(b)` — a method on the destination |
| `guid.FromBase62(s)` | `g.FromBase62(s)` / `l.FromBase62(s)` |
| `guid.FromT(t)` | `g.FromTime(clock, t)` / `l.FromTime(clock, t)` |
| `Chronos.L()` | `Chronos.Node()` — renamed, `L` is now a type |
| — | `Chronos.Order()` — new, the direction of the clock's time domain; `FromTime` needs it to place an instant into the keyspace |
| `guid.G(clock, drift...)`, `guid.Z(drift...)`, `guid.FromT(t, drift...)` | drift moved onto the clock: `guid.NewClock(guid.WithDrift(d))`, `Chronos.Drift()` |
| `guid.WithUnique(...)` | removed, see below |

Four behavioural changes come with it:

* **⟨𝒕⟩ and ⟨𝒔⟩ are always allocated as a pair.** `WithUnique` supplied ⟨𝒔⟩ from a generator independent of ⟨𝒕⟩, which opts out of the coupling that makes values sort in allocation order; it was deprecated in v2 and is gone in v3. `NewClockMock` still pins ⟨𝒕,𝒔⟩ to ⟨0,0⟩ for tests that need a fixed value.
* **⟨𝒅⟩ drift is configured on the clock, not per allocation.** v2 accepted an optional `drift ...time.Duration` on every allocator, while correctness requires the drift to be constant across a keyspace — ⟨𝒅⟩ is the most significant faction, so mixing drifts sorts values by their configuration instead of their time. v3 binds it to `Chronos` with `guid.WithDrift(...)`, which makes the mixed keyspace unrepresentable within one clock. The ladder was also re-based: `guid.WithDrift` now takes a `guid.Drift` rung rather than a duration — use `guid.DriftOf(d)` to convert one — and four of its eight rungs were moved below one second, down to Δ = 1.05 ms. The old floor of 34.36 s was an artifact of assembling the value with hand-placed shifts, which required ⟨𝒍⟩ to keep `𝑫 - 18` bits above the machine word boundary; the fractions are now placed positionally, so the schema's own range is reachable.
* **`Epoch` reports wall clock time, whichever way the clock runs.** v2 offered `EpochT` and `EpochI`, and the caller had to know which one matched the clock that allocated the value — picking wrong returned a date centuries off, silently. The direction of a clock is a decision about how the keyspace is laid out, not a fact about the event, so it must not change the reported instant. v3 has a single `uid.Epoch()`: it recovers the domain from ⟨𝒕⟩ itself, since a descending tick is `MaxUint64 - UnixNano` and therefore `>= 2^63` exactly while an ascending one is below it. The discrimination inverts on 2262-04-11, the day `int64` nanoseconds overflow, so it expires with the return type rather than before it. Ordering questions are answered by `Before`, `After` and `Time()` as before.
* **JSON no longer carries a shape marker.** v2 prefixed a local value with `*` so that a `guid.K` could round-trip as either shape. The types are distinct now, so both marshal to a plain 16-character string. v2 JSON containing `*`-prefixed values does not decode.

## Getting started

The latest version of the library is available at `main` branch. All development, including new features and bug fixes, take place on the `main` branch using forking and pull requests as described in contribution guidelines. The stable version is available via Golang modules.

Use `go get` to retrieve the library and add it as dependency to your application.

```bash
go get github.com/fogfish/guid/v3
```

Here is minimal example:

```go
package main

import (
  "fmt"
  "time"

  "github.com/fogfish/guid/v3"
)

func useDefaultClock() {
  a := guid.NewG(guid.Clock)
  time.Sleep(1 * time.Second)
  b := guid.NewG(guid.Clock)
  fmt.Printf("%s < %s is %v\n", a, b, a.Before(b))
}

func useCustomClock() {
  clock := guid.NewClock(
    guid.WithNodeID(0xffffffff),
  )

  c := guid.NewG(clock)
  time.Sleep(1 * time.Second)
  d := guid.NewG(clock)
  fmt.Printf("%s < %s is %v\n", c, d, c.Before(d))
}

func main() {
  useDefaultClock()
  useCustomClock()
}
```

The library [api specification](http://godoc.org/github.com/fogfish/guid) is available via Go doc.

### The example command

[`examples/guid`](examples/guid/main.go) is a runnable generator — it allocates identifiers of any of the three types and writes them to stdout, one per line.

```bash
go run ./examples/guid -x -n 42 -t 5ms -c 20
```

| flag | |
|---|---|
| `-l` `-g` `-x` | which type to allocate; `-g` by default, at most one |
| `-n` | node identity ⟨𝒍⟩, random when not given |
| `-t` | sleep a random interval in (0, t] between allocations |
| `-c` | how many to allocate, `0` for no limit |

Only the identifiers go to stdout, so the output pipes. Run two instances side by side with different `-n` to see the property the schema exists for — each allocator's output is a contiguous, individually ordered run of the key space:

```bash
go run ./examples/guid -x -c 1000 2>/dev/null | sort -c && echo "allocated in sort order"
```

## How To Contribute

The library is [Apache 2.0](LICENSE) licensed and accepts contributions via GitHub pull requests:

1. Fork it
2. Create your feature branch (`git checkout -b my-new-feature`)
3. Commit your changes (`git commit -am 'Added some feature'`)
4. Push to the branch (`git push origin my-new-feature`)
5. Create new Pull Request


The build and testing process requires [Go](https://golang.org) version 1.13 or later.

**Build** and **run** in your development console.

```bash
git clone https://github.com/fogfish/guid
cd guid
go test
```

## License

[![See LICENSE](https://img.shields.io/github/license/fogfish/guid.svg?style=for-the-badge)](LICENSE)


## References

1. [Lamport timestamps](https://en.wikipedia.org/wiki/Lamport_timestamps)
2. [Universal Unique Identifiers](https://tools.ietf.org/html/rfc4122),
3. [Twitter Snowflake](https://blog.twitter.com/engineering/en_us/a/2010/announcing-snowflake.html)
4. [Flake](https://github.com/boundary/flake)
