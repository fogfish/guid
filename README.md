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

Every byte counts when application is processing or storing large volume of events. This library implements fixed size 96-bit identity schema, which is castable to 64-bit under certain occasion. It is about 25% improvement to compare with UUID or similar 128-bit identity schemas (only Twitters Snowflake is 64-bit).

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

↣ ⟨𝒅⟩ is 3 drift bits defines the width Δ of the window inside which ⟨𝒍⟩ outranks time. It shows the value of less important faction of time. The value supports step-wise drift from 34 seconds to 73 minutes, configured on the clock with `guid.WithDrift(...)` and defaulting to about 4.5 minutes.

**Read Δ as a failover budget, not as a clock-skew budget.** It has to cover the interval between a silent failure and the moment the cluster has converged on a new owner — because that is the interval during which two allocators write to the same range and you need their output kept apart:

| Δ | covers |
|---|---|
| 34 s — 137 s | gossip / phi-accrual failure detection, automated lease expiry |
| 275 s *(default)* — 1099 s | slow membership convergence, cross-region hand-over |
| 2199 s — 4398 s | human-in-the-loop failover |

The same number is also the clock disagreement the ordering tolerates, which is why one knob serves both: two nodes whose clocks differ by less than Δ still sort into the same window. That matters because the target is not a managed cluster with datacenter NTP. On uncoordinated nodes — hardware without an RTC, devices behind firewalls that block NTP, VMs resuming from a snapshot, phones returning from airplane mode — tens of seconds of disagreement is the distribution, not a pathology. A timestamp-primary schema answers this by making an NTP server the coordinating authority, which is the coordination the library set out to avoid.

The drift must be the same for every value of a keyspace — ⟨𝒅⟩ is the most significant faction, so values allocated with different drift are segregated rather than interleaved. This is why it is a property of the clock and not an argument of `NewG` / `NewL`.

```go
clock := guid.NewClock(guid.WithDrift(60 * time.Second))
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

## Types

The library defines two types, each occupying exactly the bits its schema needs, so that an application storing millions of identifiers pays for nothing it does not use.

| type | schema | size | alignment | allocator |
|---|---|---|---|---|
| `guid.G` | ⟨𝒅,𝒕,𝒍,𝒔⟩ | **12 bytes** | 1 | `guid.NewG(clock)` |
| `guid.L` | ⟨𝒅,𝒕,𝒔⟩   | **8 bytes**  | 8 | `guid.NewL(clock)` |

`guid.G` holds the big-endian representation of the 96-bit number the schema defines, and nothing else. The representation is the one the schema is defined in, so memory I/O costs nothing:

* a `[]guid.G` packs without padding, 12 bytes per value;
* the wire format is the value itself — `g[:]` is a valid encoding and `copy(g[:], buf)` a valid decoding, neither shifts a single bit;
* the byte order is the order of the identifiers, so `bytes.Compare` over the raw bytes agrees with `g.Before`. An index that sorts the stored bytes — a B-tree, a sorted file, a key-value store — orders the identifiers correctly without decoding them.

`guid.L` is the 64-bit number itself, one machine word: passed in registers, compared with a single instruction, stored in 8 bytes.

The two are distinct types, which is deliberate — a local and a global value are not comparable, and the compiler now says so. Cast between them with `l.ToG(clock)` and `g.ToL()`, both of which preserve the ⟨𝒕,𝒔⟩ fraction exactly.

### Which one to use

**Reach for `guid.L` when you need a sortable key in a database.** It is 8 bytes — half of a UUID, a third smaller than `guid.G`, and the same width as a `BIGINT` — and it is *strictly* ordered rather than k-ordered: values are returned in exactly the order they were allocated, at any rate, and across a clock that NTP steps backwards. There is no drift window to reason about because there is no ⟨𝒍⟩ fraction for time to rank against. For surrogate keys, event ids and sequence numbers this is the better instrument, and the cheaper one.

Its limit is in the name: a local value is unique within the allocator that issued it. Use it where the surrounding context already disambiguates — a per-tenant or per-partition key space, a single writer, or a row that already carries the node — and use `guid.G` where it does not.

**Reach for `guid.G` when identifiers are allocated by many uncoordinated nodes** and you want the topology in the key: a contiguous, exactly ordered run per allocator, so that overlapping writers can be told apart and reconciled. That is what the extra 4 bytes buy.

```go
g := guid.NewG(guid.Clock)   // 96-bit, globally unique
l := guid.NewL(guid.Clock)   // 64-bit, unique within this allocator

g.Time()  g.Node()  g.Seq()  g.EpochT()
g.Before(other)  g.After(other)  g.Equal(other)
g.String()  g.Base62()  g.Bytes()

l.ToG(guid.Clock)  // 64-bit -> 96-bit, stamps the ⟨𝒍⟩ fraction
g.ToL()            // 96-bit -> 64-bit, drops it
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
| `guid.EpochT(uid)`, `guid.EpochI(uid)` | `uid.EpochT()`, `uid.EpochI()` |
| `guid.String(uid)`, `guid.Bytes(uid)`, `guid.Base62(uid)` | `uid.String()`, `uid.Bytes()`, `uid.Base62()` |
| `guid.FromL(clock, uid)` / `guid.ToL(uid)` | `l.ToG(clock)` / `g.ToL()` |
| `guid.FromBytes(b)` (dispatched on length) | `guid.FromBytesG(b)` / `guid.FromBytesL(b)` |
| `guid.FromBase62(s)` | `guid.FromBase62G(s)` / `guid.FromBase62L(s)` |
| `guid.FromT(t)` | `guid.FromTL(clock, t)` / `guid.FromTG(clock, t)` |
| `Chronos.L()` | `Chronos.Node()` — renamed, `L` is now a type |
| `guid.G(clock, drift...)`, `guid.Z(drift...)`, `guid.FromT(t, drift...)` | drift moved onto the clock: `guid.NewClock(guid.WithDrift(d))`, `Chronos.Drift()` |
| `guid.WithUnique(...)` | removed, see below |

Three behavioural changes come with it:

* **⟨𝒕⟩ and ⟨𝒔⟩ are always allocated as a pair.** `WithUnique` supplied ⟨𝒔⟩ from a generator independent of ⟨𝒕⟩, which opts out of the coupling that makes values sort in allocation order; it was deprecated in v2 and is gone in v3. `NewClockMock` still pins ⟨𝒕,𝒔⟩ to ⟨0,0⟩ for tests that need a fixed value.
* **⟨𝒅⟩ drift is configured on the clock, not per allocation.** v2 accepted an optional `drift ...time.Duration` on every allocator, while correctness requires the drift to be constant across a keyspace — ⟨𝒅⟩ is the most significant faction, so mixing drifts sorts values by their configuration instead of their time. v3 binds it to `Chronos` with `guid.WithDrift(...)`, which makes the mixed keyspace unrepresentable within one clock. The ladder also gained its lowest rung back, 34.36 s; that is the floor the 96-bit layout admits, since ⟨𝒍⟩ needs `𝑫 - 18` bits above the word boundary.
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
