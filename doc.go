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

Key features

This library aims important objectives:

↣ IDs allocation does not require centralized authority or coordination with
other nodes.

↣ IDs are suitable for partial event ordering in distributed environment and
helps on detection of causality violation.

↣ IDs are roughly sortable by allocation order ("time").

↣ IDs reduce indexes footprints and optimize lookup latency.


Inspiration

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

Identity Schema

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
The node identity has higher sorting priority than seconds faction of timestamp.
This allows to order events if clock drifts on nodes. The random allocation
give an application ability to introduce about 65K allocators before it meets
a high probability of collisions.

↣ ⟨𝒅⟩ is 3 drift bits defines allowed clock drift. It shows the value of less
important faction of time. The value supports step-wise drift from 30 seconds
to 36 minutes.

↣ ⟨𝒔⟩ is 14-bit of monotonic strictly locally ordered integer. It helps to
avoid collisions when multiple events happens during single millisecond or when
the clock set backwards. The 14-bit value allows to have about 16K allocations
per millisecond and over 10M per second on single node. Each instance of
application process runs a unique sequence of integers. The implementation
ensures that the same integer is not returned more than once on the current
process. Restart of the process resets the sequence.

The library supports casting of 96-bit identifier to 64-bit by dropping
⟨𝒍⟩ fraction. This optimization reduces a storage footprint if application
uses persistent allocators.

  3bit        47 bit            14 bit
   |-|------------------------|-------|
   ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩

Applications

The schema has wide range of applications where globally unique id are required.

↣ object identity: use library to generate unique identifiers.

↣ replacement of auto increment types: out-of-the-box replacement for auto
increment fields in databases.

↣ vector clock: defines a logical clock for each process.

↣ CRDTs: LWW Register, OR-set and others.

*/
package guid
