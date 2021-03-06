//
//   Copyright 2012 Dmitry Kolesnikov, All Rights Reserved
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.
//

/*

Package guid implements interface to generate k-ordered unique identifiers in lock-free and
decentralized manner for Golang applications. We says that sequence A is k-ordered if
it consists of strictly ordered subsequences of length k:

  𝑨[𝒊 − 𝒌] ≤ 𝑨[𝒊] ≤ 𝑨[𝒊 + 𝒌] for all 𝒊 such that 𝒌 < 𝒊 ≤ 𝒏−𝒌.

Key features

This library aims important objectives:

↣ IDs allocation does not require centralized authority or coordination with other nodes.

↣ IDs are suitable for partial event ordering in distributed environment and helps on
detection of causality violation.

↣ IDs are roughly sortable by allocation order ("time").

↣ IDs reduce indexes footprints and optimize lookup latency.


Inspiration

The event ordering in distributed computing is resolved using various techniques, e.g.
Lamport timestamps (https://en.wikipedia.org/wiki/Lamport_timestamps),
Universal Unique Identifiers (https://tools.ietf.org/html/rfc4122),
Twitter Snowflake (https://blog.twitter.com/engineering/en_us/a/2010/announcing-snowflake.html) and
many other techniques are offered by open source libraries.
`guid` is a Golang port of https://github.com/fogfish/uid.

All these solution made a common conclusion, globally unique ID is a triple ⟨𝒕, 𝒍, 𝒔⟩:
⟨𝒕⟩ monotonically increasing clock or timestamp is a primary dimension to roughly sort
events, ⟨𝒍⟩ is spatially unique identifier of ID allocator so called node location,
⟨𝒔⟩ sequence is a monotonic integer, which prevents clock collisions. The `guid` library
addresses few issues observed in other solutions.

Every byte counts when application is processing or storing large volume of events.
This library implements fixed size 96-bit identity schema, which is castable to 64-bit
under certain occasion. It is about 25% improvement to compare with UUID or similar 128-bit
identity schemas (only Twitters Snowflake is 64-bit).

Most of identity schemas uses monotonically increasing clock (timestamp) to roughly order
events. The resolution of clock varies from nanoseconds to milliseconds. We found that
usage of timestamp is not perfectly aligned with the goal of decentralized ID allocations.
Usage of time synchronization protocol becomes necessary at distributed systems. Strictly
speaking, NTP server becomes an authority to coordinate clock synchronization. This happens
because schemas uses time fraction ⟨𝒕⟩ as a primary sorting key. In contrast with other
libraries, `guid` do not give priority to single fraction of identity triple ⟨𝒕⟩ or ⟨𝒍⟩.
It uses dynamic schema where the location fraction has higher priority than time only at
particular precision. It allows to keep ordering consistent even if clocks on other node is
skewed.

Identity Schema

A fixed size of 96-bit is used to implement identity schema

  3bit  47 bit - 𝒅 bit         32 bit      𝒅 bit  14 bit
   |-|-------------------|----------------|-----|-------|
   ⟨𝒅⟩        ⟨𝒕⟩                ⟨𝒍⟩         ⟨𝒕⟩     ⟨𝒔⟩

↣ ⟨𝒕⟩ is 47-bit UTC timestamp with millisecond precision. It is derived from nanosecond
UNIX timesamp by shifting it by 17 bits (time.Now().UnixNano() << 17). The library is
able to change the base timestamp to any value in-order to address Year 2038 problem.

↣ ⟨𝒍⟩ is 32-bits node/allocator identifier. It is allocated randomly to each node using
cryptographic random generator or application provided value. The node identity has higher
sorting priority than seconds faction of timestamp. This allows to order events if clock
drifts on nodes. The random allocation give an application ability to introduce about 65K
allocators before it meets a high probability of collisions.

↣ ⟨𝒅⟩ is 3 drift bits defines allowed clock drift. It shows the value of less important
faction of time. The value supports step-wise drift from 30 seconds to 36 minutes.

↣ ⟨𝒔⟩ is 14-bit of monotonic strictly locally ordered integer. It helps to avoid collisions
when multiple events happens during single millisecond or when the clock set backwards.
The 14-bit value allows to have about 16K allocations per millisecond and over 10M per second
on single node. Each instance of application process runs a unique sequence of integers.
The implementation ensures that the same integer is not returned more than once on the current
process. Restart of the process resets the sequence.

The library supports casting of 96-bit identifier to 64-bit by dropping ⟨𝒍⟩ fraction. This
optimization reduces a storage footprint if application uses persistent allocators.

  3bit        47 bit            14 bit
   |-|------------------------|-------|
   ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩

Applications

The schema has wide range of applications where globally unique id are required.

↣ object identity: use library to generate unique identifiers.

↣ replacement of auto increment types: out-of-the-box replacement for auto increment
fields in relational databases.

↣ vector clock: defines a logical clock for each process.

↣ CRDTs: LWW Register, OR-set and others.

*/
package guid

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"sync/atomic"
	"time"
)

/*

G is native representation of k-ordered number (global format).
It is 96-bit long and requires no central registration process.
Note: Golang struct is 128-bits but only 96-bits are used effectively.
The serialization process ensures that only 96-bits are used.
*/
type G struct{ hi, lo uint64 }

/*

L is native representation of k-ordered number (local format).
*/
type L struct{ lo uint64 }

/*

Seq is a global default allocator of unique IDs
*/
var Seq Alloc = New()

/*

Unique Monotonic Integer sequence
*/
var unique int64

/*

T is a timestamp generator functions
*/
type T func() uint64

/*

Alloc type defines behavior of UID allocation
*/
type Alloc struct {
	uint64
	now T
}

/*

Config is a base type of allocation configuration. Allocators allows an application to
define custom strategies to generate ⟨𝒍⟩ location or ⟨𝒕⟩ timestamp.
*/
type Config func(*Alloc)

/*

Clock configures a custom timestamp generator functions
*/
func Clock(t T) Config {
	return func(n *Alloc) {
		n.now = t
	}
}

/*

Allocator configures an application defined allocator ID
*/
func Allocator(node uint64) Config {
	return func(n *Alloc) {
		n.uint64 = node & 0x00000000ffffffff
	}
}

/*

NamedAllocator configures a constant allocator ID. The value is obtained from
CONFIG_ALLOCATOR_ID environment variable
*/
func NamedAllocator() Config {
	return func(n *Alloc) {
		h := sha256.New()
		h.Write([]byte(os.Getenv("CONFIG_ALLOCATOR_ID")))
		hash := h.Sum(nil)
		n.uint64 = uint64(hash[0])<<24 | uint64(hash[1])<<16 | uint64(hash[2])<<8 | uint64(hash[3])
	}
}

/*

DefaultClock configures default timestamp generator functions derived from
time.Now().UnixNano()
*/
func DefaultClock() Config {
	return func(n *Alloc) {
		n.now = func() uint64 {
			return uint64(time.Now().UnixNano())
		}
	}
}

/*

DefaultAllocator configures default algorithm to derive allocator name
from cryptographic random generator.
*/
func DefaultAllocator() Config {
	return func(n *Alloc) {
		rander := rand.Reader
		bytes := make([]byte, 8)
		if _, err := io.ReadFull(rander, bytes); err != nil {
			panic(err.Error())
		}

		node := uint64(0x0)
		for i, b := range bytes {
			node = node | uint64(b)<<(64-8*(i+1))
		}
		n.uint64 = node & 0x00000000ffffffff
	}
}

/*

New creates a new instance of ID allocator
*/
func New(opts ...Config) Alloc {
	node := Alloc{}
	spec := []Config{DefaultClock(), DefaultAllocator()}
	for _, f := range append(spec, opts...) {
		f(&node)
	}
	return node
}

/*

Z returns "zero" unique 64-bit k-order identifier.
*/
func (n Alloc) Z(interval ...time.Duration) (uid L) {
	d := (drift(interval...) - driftZ) << 61
	uid.lo = d
	return
}

/*

L generates locally unique 64-bit k-order identifier.

  3bit        47 bit           14 bit
  |-|------------------------|-------|
  ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩

*/
func (n Alloc) L(interval ...time.Duration) L {
	return newL(drift(interval...), n.now(), uniqueInt())
}

func newL(drift, t, seq uint64) (uid L) {
	d := (drift - driftZ) << 61
	x := t >> (14 + 3) << 14

	uid.lo = d | x | seq
	return
}

/*

G generate globally unique 96-bit k-order identifier.

  3bit  47 bit - 𝒅 bit         32 bit     𝒅 bit  14 bit
  |-|-------------------|----------------|-----|-------|
  ⟨𝒅⟩        ⟨𝒕⟩                ⟨𝒍⟩         ⟨𝒕⟩     ⟨𝒔⟩

*/
func (n Alloc) G(interval ...time.Duration) G {
	return newG(n.uint64, drift(interval...), n.now(), uniqueInt())
}

func newG(n, drift, t, seq uint64) (uid G) {
	thi, tlo := splitT(t, drift)
	nhi, nlo := splitNode(n, drift)

	uid.hi = thi | nhi
	uid.lo = nlo | tlo | seq

	return
}

//
func uniqueInt() uint64 {
	return uint64(atomic.AddInt64(&unique, 1) & 0x3fff)
}

//
const driftZ = 18

func drift(interval ...time.Duration) uint64 {
	switch {
	case len(interval) == 0:
		return driftZ + 3
	case interval[0] <= 34*time.Second:
		return driftZ
	case interval[0] <= 68*time.Second:
		return driftZ + 1
	case interval[0] <= 137*time.Second:
		return driftZ + 2
	case interval[0] <= 274*time.Second:
		return driftZ + 3
	case interval[0] <= 549*time.Second:
		return driftZ + 4
	case interval[0] <= 1099*time.Second:
		return driftZ + 5
	case interval[0] <= 2199*time.Second:
		return driftZ + 6
	default:
		return driftZ + 7
	}
}

//
func splitT(t uint64, drift uint64) (uint64, uint64) {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	// 14 bits of time is exchange for seq
	//  3 bits is reserved for drift
	x := t >> (14 + 3)
	a := 64 - 14 - drift
	b := 32 - a

	lo := (x << (a + 14)) >> a
	hi := (x >> drift) << b
	dd := (drift - driftZ) << 29

	return hi | dd, lo
}

func splitNode(node, drift uint64) (uint64, uint64) {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	a := 64 - 14 - drift
	b := 32 - a

	lo := node << (drift + 14)
	hi := node >> (32 - b)

	return hi, lo
}

/*

ID generates new k-order value and encodes it to string
*/
func (n Alloc) ID() string {
	return n.G().String()
}

/*

ToG casts local (64-bit) k-order UID to global (96-bit) one
*/
func (uid L) ToG(n Alloc) G {
	d := (uid.lo >> 61) + driftZ
	return newG(n.uint64, d, uint64(uid.Time()), uid.Seq())
}

/*

ToL casts global (96-bit) k-order value to local (64-bit) one
*/
func (uid G) ToL() L {
	d := (uid.hi >> 29) + driftZ
	return newL(d, uint64(uid.Time()), uid.Seq())
}

/*

Time returns ⟨𝒕⟩ timestamp fraction from identifier.
The returned value is nano seconds compatible with time.Unix(0, uid.Time())
*/
func (uid L) Time() int64 {
	return int64(uid.lo << 3 >> (14 + 3) << (14 + 3))
}

/*

Time returns ⟨𝒕⟩ timestamp fraction from identifier.
The returned value is nano seconds compatible with time.Unix(0, uid.Time())
*/
func (uid G) Time() int64 {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	d := (uid.hi >> 29) + driftZ
	a := 64 - 14 - d
	b := 32 - a

	hi := (uid.hi >> b) << d
	lo := (uid.lo << a) >> (64 - d)

	t := ((hi | lo) << (14 + 3))

	return int64(t) & 0x7fffffffffffffff
}

/*

Node returns ⟨𝒍⟩ location fraction from identifier.
*/
func (uid G) Node() uint64 {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	d := (uid.hi >> 29) + driftZ
	a := 64 - 14 - d
	b := 32 - a

	lo := uid.lo >> (d + 14)
	hi := uid.hi << (64 - b) >> (64 - b - a)

	return hi | lo
}

/*

Seq returns ⟨𝒔⟩ sequence value. The value of monotonic unique integer at the time of ID creation.
*/
func (uid L) Seq() uint64 {
	return uid.lo & 0x3fff
}

/*

Seq returns ⟨𝒔⟩ sequence value. The value of monotonic unique integer at the time of ID creation.
*/
func (uid G) Seq() uint64 {
	return uid.lo & 0x3fff
}

/*

Eq compares k-order UIDs, returns true if values are equal
*/
func (uid L) Eq(b L) bool {
	return uid.lo == b.lo
}

/*

Eq compares k-order UIDs, returns true if values are equal
*/
func (uid G) Eq(b G) bool {
	return uid.hi == b.hi && uid.lo == b.lo
}

/*

Lt compares k-order UIDs, return true if value uid (this) less
than value b (argument)
*/
func (uid L) Lt(b L) bool {
	return uid.lo < b.lo
}

/*

Lt compares k-order UIDs, return true if value uid (this) less
than value b (argument)
*/
func (uid G) Lt(b G) bool {
	return uid.hi <= b.hi && uid.lo < b.lo
}

/*

Diff approximates distance between k-order UIDs.
*/
func (uid L) Diff(b L) L {
	t := uint64(uid.Time() - b.Time())
	s := uid.Seq() - b.Seq()
	d := (uid.lo >> 61) + driftZ
	return newL(d, t, s)
}

/*

Diff approximates distance between k-order UIDs.
*/
func (uid G) Diff(b G) G {
	t := uint64(uid.Time() - b.Time())
	s := uid.Seq() - b.Seq()
	d := (uid.hi >> 29) + driftZ
	return newG(uid.Node(), d, t, s)
}

/*

Split decomposes UID value to bytes slice. The funcion acts as binary comprehension,
the value n defines number of bits to extract into each cell.
*/
func (uid L) Split(n uint64) (bytes []byte) {
	return split(0, uid.lo, 64, n)
}

/*

Split decomposes UID value to bytes slice. The funcion acts as binary comprehension,
the value n defines number of bits to extract into each cell.
*/
func (uid G) Split(n uint64) (bytes []byte) {
	return split(uid.hi, uid.lo, 96, n)
}

func split(hi, lo, size, n uint64) (bytes []byte) {
	hilo := uint64(64) // hi | lo division at
	bytes = make([]byte, size/n)

	mask := uint64(1<<n) - 1
	i := 0

	for a := size; a >= n; a -= n {
		b := a - n
		switch {
		case a >= hilo && b >= hilo:
			value := byte(hi >> (b - hilo) & mask)
			bytes[i] = value
		case a <= hilo && b <= hilo:
			value := byte(lo >> b & mask)
			bytes[i] = value
		case a > hilo && b < hilo:
			suffix := uint64(1<<(a-hilo)) - 1
			hi := byte(hi & suffix)
			lo := byte(lo >> b)
			bytes[i] = hi<<(hilo-b) | lo
		}
		i++
	}

	return
}

/*

Fold composes UID value from byte slice. The operation is inverse to Split.
*/
func (uid *L) Fold(n uint64, bytes []byte) {
	_, uid.lo = fold(64, n, bytes)
}

/*

Fold composes UID value from byte slice. The operation is inverse to Split.
*/
func (uid *G) Fold(n uint64, bytes []byte) {
	uid.hi, uid.lo = fold(96, n, bytes)
}

func fold(size, n uint64, bytes []byte) (hi, lo uint64) {
	hilo := uint64(64)

	mask := uint64(1<<n) - 1
	i := 0

	for a := size; a >= n; a -= n {
		b := a - n
		switch {
		case a >= hilo && b >= hilo:
			hi |= (uint64(bytes[i]) & mask) << (b - hilo)
		case a <= hilo && b <= hilo:
			lo |= (uint64(bytes[i]) & mask) << b
		case a > hilo && b < hilo:
			hi |= (uint64(bytes[i]) & mask) >> (hilo - b)
			lo |= (uint64(bytes[i]) & mask) << b
		}
		i++
	}
	return
}

/*

Bytes encodes k-odered value to byte slice
*/
func (uid L) Bytes() []byte {
	return uid.Split(8)
}

/*

Bytes encodes k-odered value to byte slice
*/
func (uid G) Bytes() []byte {
	return uid.Split(8)
}

/*

String encodes k-ordered value to lexicographically sortable strings
*/
func (uid L) String() string {
	// Note: split only works if result is aligned to divider
	//       96 ÷ 6 = 16
	//       64 ÷ 6 = 10 rem 1
	return encode64(uid.Split(4))
}

/*

String encodes k-ordered value to lexicographically sortable strings
*/
func (uid G) String() string {
	return encode64(uid.Split(6))
}

/*

FromBytes decodes converts k-order UID from bytes
*/
func (uid *L) FromBytes(val []byte) {
	uid.Fold(8, val)
}

/*

FromBytes decodes converts k-order UID from bytes
*/
func (uid *G) FromBytes(val []byte) {
	uid.Fold(8, val)
}

/*

FromString decodes converts k-order UID from lexicographically sortable strings
*/
func (uid *L) FromString(val string) {
	// Note: split only works if result is aligned to divider
	//       96 ÷ 6 = 16
	//       64 ÷ 6 = 10 rem 1 (thus divider 4)
	uid.Fold(4, decode64(val))
}

/*

FromString decodes converts k-order UID from lexicographically sortable strings
*/
func (uid *G) FromString(val string) {
	uid.Fold(6, decode64(val))
}

/*

UnmarshalJSON decodes lexicographically sortable strings to UID value
*/
func (uid *L) UnmarshalJSON(b []byte) (err error) {
	var val string
	if err = json.Unmarshal(b, &val); err != nil {
		return
	}
	uid.FromString(val)
	return
}

/*

UnmarshalJSON decodes lexicographically sortable strings to UID value
*/
func (uid *G) UnmarshalJSON(b []byte) (err error) {
	var val string
	if err = json.Unmarshal(b, &val); err != nil {
		return
	}
	uid.FromString(val)
	return
}

/*

MarshalJSON encodes k-ordered value to lexicographically sortable JSON strings
*/
func (uid L) MarshalJSON() (bytes []byte, err error) {
	return json.Marshal(uid.String())
}

/*

MarshalJSON encodes k-ordered value to lexicographically sortable JSON strings
*/
func (uid G) MarshalJSON() (bytes []byte, err error) {
	return json.Marshal(uid.String())
}
