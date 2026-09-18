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
is also 64-bit.

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
pathology. It shows the value of less important faction of time. The value supports
step-wise drift from 34 seconds to 73 minutes, configured on the clock with
WithDrift and defaulting to about 4.5 minutes. The drift must be the same for
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

# Types

The library defines two types, each occupying exactly the bits its schema
needs, so that an application storing millions of identifiers pays for nothing
it does not use:

	guid.G  96-bit globally unique value, 12 bytes, allocated by NewG
	guid.L  64-bit locally unique value,   8 bytes, allocated by NewL

	g := guid.NewG(guid.Clock)
	l := guid.NewL(guid.Clock)

G holds the big-endian representation of the 96-bit number the schema defines,
and nothing else. The type is therefore 12 bytes wide with single byte
alignment, so a []G packs without padding; the wire format is the value itself,
g[:] is a valid encoding and copy(g[:], buf) a valid decoding; and the byte
order is the order of the identifiers, so bytes.Compare over the raw bytes and
g.Before agree. An index that sorts the stored bytes orders the identifiers
correctly without decoding them.

L is the 64-bit number itself, one machine word, passed in registers and
compared with a single instruction.

The two are distinct types, which is deliberate: a local value and a global
value are not comparable, and the compiler now says so. Cast between them with
l.ToG(clock) and g.ToL(), both of which preserve the ⟨𝒕,𝒔⟩ fraction exactly.

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
is what the extra 4 bytes buy.

# Applications

The schema has wide range of applications where globally unique id are required.

↣ object identity: use library to generate unique identifiers.

↣ replacement of auto increment types: out-of-the-box replacement for auto
increment fields in databases.

↣ vector clock: defines a logical clock for each process.

↣ CRDTs: LWW Register, OR-set and others.
*/
package guid
