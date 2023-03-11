<p align="center">
  <h3 align="center">GUID</h3>
  <p align="center"><strong>K-ordered unique identifiers in lock-free and
decentralized manner for Golang applications</strong></p>

  <p align="center">
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

* IDs allocation does not require centralized authority or coordination with other nodes.
* IDs are suitable for partial event ordering in distributed environment and helps on detection of causality violation.
* IDs are roughly sortable by allocation order ("time").
* IDs reduce indexes footprints and optimize lookup latency.


## Inspiration

The event ordering in distributed computing is resolved using various techniques, e.g. Lamport timestamps, Universal Unique Identifiers, Twitter Snowflake and many other techniques are offered by open source libraries. `guid` is a Golang port of [Erlang's uid library](https://github.com/fogfish/uid).

All these solution made a common conclusion, globally unique ID is a triple ⟨𝒕, 𝒍, 𝒔⟩: ⟨𝒕⟩ monotonically increasing clock or timestamp is a primary dimension to roughly sort events, ⟨𝒍⟩ is spatially unique identifier of ID allocator so called node location, ⟨𝒔⟩ sequence is a monotonic integer, which prevents clock collisions. The `guid` library addresses few issues observed in other solutions.

Every byte counts when application is processing or storing large volume of events. This library implements fixed size 96-bit identity schema, which is castable to 64-bit under certain occasion. It is about 25% improvement to compare with UUID or similar 128-bit identity schemas (only Twitters Snowflake is 64-bit).

Most of identity schemas uses monotonically increasing clock (timestamp) to roughly order events. The resolution of clock varies from nanoseconds to milliseconds. We found that usage of timestamp is not perfectly aligned with the goal of decentralized ID allocations. Usage of time synchronization protocol becomes necessary at distributed systems. Strictly speaking, NTP server becomes an authority to coordinate clock synchronization. This happens because schemas uses time fraction ⟨𝒕⟩ as a primary sorting key. In contrast with other libraries, `guid` do not give priority to single fraction of identity triple ⟨𝒕⟩ or ⟨𝒍⟩. It uses dynamic schema where the location fraction has higher priority than time only at particular precision. It allows to keep ordering consistent even if clocks on other node is skewed.

## Identity Schema

A fixed size of 96-bit is used to implement identity schema

```
  3bit  47 bit - 𝒅 bit         32 bit      𝒅 bit  14 bit
   |-|-------------------|----------------|-----|-------|
   ⟨𝒅⟩        ⟨𝒕⟩                ⟨𝒍⟩         ⟨𝒕⟩     ⟨𝒔⟩
```

↣ ⟨𝒕⟩ is 47-bit UTC timestamp with millisecond precision. It is derived from nanosecond UNIX timestamp by shifting it by 17 bits (time.Now().UnixNano() << 17). The library is able to change the base timestamp to any value in-order to address Year 2038 problem.

↣ ⟨𝒍⟩ is 32-bits node/allocator identifier. It is allocated randomly to each node using cryptographic random generator or application provided value. The node identity has higher sorting priority than seconds faction of timestamp. This allows to order events if clock drifts on nodes. The random allocation give an application ability to introduce about 65K allocators before it meets a high probability of collisions.

↣ ⟨𝒅⟩ is 3 drift bits defines allowed clock drift. It shows the value of less important faction of time. The value supports step-wise drift from 30 seconds to 36 minutes.

↣ ⟨𝒔⟩ is 14-bit of monotonic strictly locally ordered integer. It helps to avoid collisions when multiple events happens during single millisecond or when the clock set backwards. The 14-bit value allows to have about 16K allocations per millisecond and over 10M per second on single node. Each instance of application process runs a unique sequence of integers. The implementation ensures that the same integer is not returned more than once on the current
process. Restart of the process resets the sequence.

The library supports casting of 96-bit identifier to 64-bit by dropping ⟨𝒍⟩ fraction. This optimization reduces a storage footprint if application uses persistent allocators.

```
  3bit        47 bit            14 bit
   |-|------------------------|-------|
   ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩
```

## Getting started

The latest version of the library is available at `main` branch. All development, including new features and bug fixes, take place on the `main` branch using forking and pull requests as described in contribution guidelines. The stable version is available via Golang modules.

Use `go get` to retrieve the library and add it as dependency to your application.

```bash
go get github.com/fogfish/guid
```

Here is minimal example (also available in [playground](https://go.dev/play/p/zaIM7BXGt8F)):

```go
package main

import (
  "fmt"
  "time"

  "github.com/fogfish/guid/v2"
)

func useDefaultClock() {
  a := guid.G(guid.Clock)
  time.Sleep(1 * time.Second)
  b := guid.G(guid.Clock)
  fmt.Printf("%s < %s is %v\n", a, b, guid.Before(a, b))
}

func useCustomClock() {
  clock := guid.NewClock(
    guid.WithNodeID(0xffffffff),
  )

  c := guid.G(clock)
  time.Sleep(1 * time.Second)
  d := guid.G(clock)
  fmt.Printf("%s < %s is %v\n", c, d, guid.Before(c, d))
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
