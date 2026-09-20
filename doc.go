/*

  Copyright 2012 Dmitry Kolesnikov, All Rights Reserved

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

/*
Package guid implements interface to generate k-ordered unique identifiers in
lock-free and decentralized manner for Golang applications. We says that
sequence A is k-ordered if it consists of strictly ordered subsequences of
length k:

	𝑨[𝒊 − 𝒌] ≤ 𝑨[𝒊] ≤ 𝑨[𝒊 + 𝒌] for all 𝒊 such that 𝒌 < 𝒊 ≤ 𝒏−𝒌.

# Key features

This library aims important objectives:

↣ IDs allocation does not require centralized authority or coordination with
other nodes.

↣ IDs are suitable for partial event ordering in distributed environment and
helps on detection of causality violation.

↣ IDs are roughly sortable by allocation order ("time").

↣ IDs reduce indexes footprints and optimize lookup latency.

# Inspiration

The event ordering in distributed computing is resolved using various
techniques, e.g. Lamport timestamps (https://en.wikipedia.org/wiki/Lamport_timestamps),
Universal Unique Identifiers (https://tools.ietf.org/html/rfc4122),
Twitter Snowflake (https://blog.twitter.com/engineering/en_us/a/2010/announcing-snowflake.html)
and many other techniques are offered by open source libraries.
`guid` is a Golang port of https://github.com/fogfish/uid.

All these solution made a common conclusion, globally unique ID is a triple ⟨𝒕, 𝒍, 𝒔⟩:

↣ ⟨𝒕⟩ monotonically increasing clock or timestamp is a primary dimension
to roughly sort events,

↣ ⟨𝒍⟩ is spatially unique identifier of ID allocator so called node location,

↣ ⟨𝒔⟩ sequence is a monotonic integer, which prevents clock collisions.
The value is global for the node

The `guid` library addresses few issues observed in other solutions.

Every byte counts when application is processing or storing large volume of
events. This library implements fixed size 96-bit identity schema, which is
castable to 64-bit under certain occasion. It is about 25% improvement to
compare with UUID or similar 128-bit identity schemas. Twitters Snowflake
is also 64-bit. The same schema is also offered at 128 bits, as an RFC 9562
UUIDv8, for deployments where interoperability outweighs the footprint.

Most of identity schemas uses monotonically increasing clock (timestamp) to
roughly order events. The resolution of clock varies from nanoseconds to
milliseconds. We found that usage of timestamp is not perfectly aligned with
the goal of decentralized ID allocations. Usage of time synchronization protocol
becomes necessary at distributed systems. Strictly speaking, NTP server becomes
an authority to coordinate clock synchronization. This happens because schemas
uses time fraction ⟨𝒕⟩ as a primary sorting key. In contrast with other
libraries, `guid` do not give priority to single ⟨𝒕⟩ or ⟨𝒍⟩ fraction of
identity triple. It uses dynamic schema where the location fraction has higher
priority than time only at particular precision. It allows to keep ordering
consistent even if clocks on other node is skewed.

# Identity Schema

A fixed size of 96-bit is used to implement identity schema

	3bit  47 bit - 𝒅 bit         32 bit      𝒅 bit  14 bit
	 |-|-------------------|----------------|-----|-------|
	 ⟨𝒅⟩        ⟨𝒕⟩                ⟨𝒍⟩         ⟨𝒕⟩     ⟨𝒔⟩

↣ ⟨𝒕⟩ is 47-bit UTC timestamp with millisecond precision. It is derived from
nanosecond UNIX timesamp by shifting it by 17 bits (time.Now().UnixNano() >> 17).
The library is able to change the base timestamp to any value in-order to
address Year 2038 problem.

↣ ⟨𝒍⟩ is 32-bits node/spacial identifier. It is allocated randomly to each
node using cryptographic random generator or application provided value.
The node identity has higher sorting priority than the low bits of the
timestamp, which is what makes each allocator's output a contiguous, exactly
ordered run of the key space. If ⟨𝒍⟩ is meant to carry topology, assign it with
WithNodeID rather than leaving the random default: random identities sort
arbitrarily, so adjacent ring positions land far apart. The random allocation
give an application ability to introduce about 65K allocators before it meets
a high probability of collisions.

↣ ⟨𝒅⟩ is 3 drift bits defines the width of the window inside which ⟨𝒍⟩
outranks time. Read it as a failover budget rather than as a clock-skew budget:
it has to cover the interval between a silent failure and the moment the
cluster has converged on a new owner, because that is the interval during which
two allocators write to the same range. The same number is also the clock
disagreement the ordering tolerates, which is why one knob serves both — and on
uncoordinated nodes, hardware without an RTC or devices behind firewalls that
block NTP, tens of seconds of disagreement is the distribution rather than a
pathology. It shows the value of less important faction of time. The code
selects a rung of an eight step ladder, Drift131us to Drift39h, which runs from
131 µs to 39 hours, configured on the clock with WithDrift and defaulting to
Drift275s, about 4.5 minutes. Use DriftOf to pick the smallest rung that covers
a budget expressed as a duration. The drift must be the same for
every value of a keyspace: ⟨𝒅⟩ is the most significant faction, so values
allocated with different drift are segregated rather than interleaved. This is
why it is a property of Chronos and not an argument of NewG and NewL.

↣ ⟨𝒔⟩ is 14-bit of monotonic strictly locally ordered integer. It helps to
avoid collisions when multiple events happens during single millisecond or when
the clock set backwards. The 14-bit value allows to have about 16K allocations
per tick of ⟨𝒕⟩ and over 100M per second on single node. Each instance of
application process runs a unique sequence of integers. The implementation
ensures that the same integer is not returned more than once on the current
process. Restart of the process resets the sequence.

⟨𝒕⟩ and ⟨𝒔⟩ are allocated together, as a single atomic step, so that the pair
strictly increases with every allocation. ⟨𝒔⟩ counts within one tick of ⟨𝒕⟩ and
restarts when the clock ticks; an allocator that exhausts a tick carries into
the next one rather than folding ⟨𝒔⟩ back to zero. Two consequences are worth
knowing:

↣ Values allocated by one process are ordered exactly as they were allocated,
whatever the allocation rate and even if the clock is stepped backwards, in
which case ⟨𝒕⟩ holds its high water mark until real time catches up.

↣ A process that saturates the allocator, a loop that allocates and discards
and does nothing else, makes ⟨𝒕⟩ run ahead of the wall clock. The ordering is
unaffected and the gap closes on its own once the loop stops. See §3.6 of
doc/proof.md for the rates.

The library supports casting of 96-bit identifier to 64-bit by dropping
⟨𝒍⟩ fraction. This optimization reduces a storage footprint if application
uses persistent allocators.

	3bit        47 bit            14 bit
	 |-|------------------------|-------|
	 ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩

The same schema is also defined at 128 bits, where ⟨𝒍⟩ is 58 bits wide and the
6 bits RFC 9562 reserves make the value a UUID of version 8, see guid.X.

	3bit  47 bit - 𝒅 bit             58 bit          𝒅 bit  14 bit
	 |-|-------------------|--------------------------|-----|-------|
	 ⟨𝒅⟩        ⟨𝒕⟩                    ⟨𝒍⟩               ⟨𝒕⟩     ⟨𝒔⟩

# Types

The library defines three types, each occupying exactly the bits its schema
needs, so that an application storing millions of identifiers pays for nothing
it does not use:

	guid.X  128-bit globally unique value, 16 bytes, allocated by NewX
	guid.G   96-bit globally unique value, 12 bytes, allocated by NewG
	guid.L   64-bit locally unique value,   8 bytes, allocated by NewL

	x := guid.NewX(guid.Clock)
	g := guid.NewG(guid.Clock)
	l := guid.NewL(guid.Clock)

All three carry the same fractions in the same order and share the same drift
ladder, clock and sequencer. They differ only in how much room is left for the
node: none for L, 32 bits for G, 58 bits for X.

G holds the big-endian representation of the 96-bit number the schema defines,
and nothing else. The type is therefore 12 bytes wide with single byte
alignment, so a []G packs without padding; the wire format is the value itself,
g[:] is a valid encoding and copy(g[:], buf) a valid decoding; and the byte
order is the order of the identifiers, so bytes.Compare over the raw bytes and
g.Before agree. An index that sorts the stored bytes orders the identifiers
correctly without decoding them.

L is the 64-bit number itself, one machine word, passed in registers and
compared with a single instruction.

X is the 128-bit case, and it spends its extra 4 bytes on being an RFC 9562
UUID of version 8 rather than on the clock. Six of the 128 bits are the version
and variant fields the RFC fixes, which leaves 122 for the schema and widens
⟨𝒍⟩ to 58 bits. The result renders, parses and stores as a UUID everywhere a
UUID does — a uuid column, a log viewer, a UUID library in another language —
instead of showing up as a malformed v7. The reserved bits are constants of the
format and therefore transparent to comparison, so the byte order is still the
order of the identifiers.

The types are distinct, which is deliberate: values of different width are not
comparable, and the compiler now says so. Values of two different types must
never share a keyspace either — they are different widths with different field
semantics, and no ordering relation between them is defined.

# Decoding and casting

Only allocation is a package function. Everything that turns data you already
hold into a value is a method on the destination, so the type you are building
is the receiver and the compiler picks the decoder:

	var uid guid.X
	err := uid.FromString("06377f2a-0cb8-8a3f-b000-0000000003e9")

	uid.FromBytes(b)     // inverse of Bytes
	uid.FromString(s)    // inverse of String
	uid.FromBase62(s)    // inverse of Base62
	uid.Fold(n, b)       // inverse of Split
	uid.FromTime(clock, t)

This is the one place the schema's cardinal rule can be broken by a typo — a
value of the wrong type decoded into a keyspace is unrecoverable — so it is not
left to a suffix on a function name that the reader has to notice. There is no
FromStringG to write instead of FromStringX; g.FromString reads a G because g
is a G.

Casts work the same way and read in the direction of the assignment:

	g.FromL(clock, l)   // 64-bit  -> 96-bit,  stamps the ⟨𝒍⟩ fraction
	g.FromX(x)          // 128-bit -> 96-bit,  truncates ⟨𝒍⟩ to 32 bits
	l.FromG(g)          // drops ⟨𝒍⟩
	l.FromX(x)          // drops ⟨𝒍⟩
	x.FromG(g)          // 96-bit  -> 128-bit, ⟨𝒍⟩ keeps the 32 bits it had
	x.FromL(clock, l)   // 64-bit  -> 128-bit, stamps the ⟨𝒍⟩ fraction

All six preserve the ⟨𝒕,𝒔⟩ fraction exactly.

A decoder that returns an error leaves the destination unchanged, so a value
that was already there survives a failed decode rather than being half
overwritten. The receiver has to be addressable, which a variable, a struct
field and a slice element are, and a map element or a function result is not —
those need a temporary.

All three types also implement encoding.TextMarshaler, TextUnmarshaler,
BinaryMarshaler and BinaryUnmarshaler, so they travel through gob, yaml, toml,
a struct tag or any other codec that speaks the stdlib interfaces without that
codec knowing this package exists. UnmarshalText is FromString and
UnmarshalBinary is FromBytes, under the names the stdlib expects.

# Which one to use

Reach for guid.L when you need a sortable key in a database. It is 8 bytes,
half of a UUID, a third smaller than guid.G and the same width as a BIGINT, and
it is strictly ordered rather than k-ordered: values are returned in exactly
the order they were allocated, at any rate, and across a clock that NTP steps
backwards. There is no drift window to reason about because there is no ⟨𝒍⟩
fraction for time to rank against. For surrogate keys, event ids and sequence
numbers it is the better instrument and the cheaper one.

Its limit is in the name: a local value is unique within the allocator that
issued it. Use it where the surrounding context already disambiguates, a
per-tenant or per-partition key space, a single writer, or a row that already
carries the node, and use guid.G where it does not.

Reach for guid.G when identifiers are allocated by many uncoordinated nodes and
the topology belongs in the key: a contiguous, exactly ordered run per
allocator, so that overlapping writers can be told apart and reconciled. That
is what the extra 4 bytes buy. Its ⟨𝒍⟩ is 32 bits, so randomly allocated node
identities meet a birthday bound at about 65 000 allocators.

Reach for guid.X when the identifiers leave Go, or when there are more
allocators than 32 bits of ⟨𝒍⟩ can keep apart. It is a real UUID, so other
systems read it without porting anything — which matters for a schema whose
premise is uncoordinated allocation, since a cluster of uncoordinated nodes is
rarely a cluster of uniform Go processes — and its 58-bit ⟨𝒍⟩ moves the
birthday bound to about 5.4·10⁸ allocators. At 16 bytes it costs the same as
the UUIDv7 it replaces, and adds sub-millisecond ordering, topology in the key,
exact intra-node sequencing and a machine-checked proof. It is not free of the
library's own conditions: X asks for a drift held constant across the cluster
and for node identities to stay distinct, where UUIDv7 asks nothing and is
unconditionally unique. In one table:

	sortable key, one allocator or a disambiguating context   guid.L    8 B
	many uncoordinated allocators, footprint matters          guid.G   12 B
	many uncoordinated allocators, interop or node count      guid.X   16 B

# Applications

The schema has wide range of applications where globally unique id are required.

↣ object identity: use library to generate unique identifiers.

↣ replacement of auto increment types: out-of-the-box replacement for auto
increment fields in databases.

↣ vector clock: defines a logical clock for each process.

↣ CRDTs: LWW Register, OR-set and others.
*/
package guid
