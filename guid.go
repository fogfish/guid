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

UID is Golang native representation of k-ordered number.
It is 96-bit long and requires no central registration process.
Note: Golang struct is 128-bits but only 96-bits are used effectively.
The serialization process ensures that only 96-bits are used.
*/
type UID struct{ hi, lo uint64 }

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

DefaultClock configures default timestamp generator functions derived from time.Now().UnixNano()
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
func (n Alloc) Z(interval ...time.Duration) (uid UID) {
	d := (drift(interval...) - 22) << 61
	uid.lo = d
	return
}

/*

L generates locally unique 64-bit k-order identifier.

  3bit        47 bit           14 bit
  |-|------------------------|-------|
  ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩

*/
func (n Alloc) L(interval ...time.Duration) UID {
	return newL(drift(interval...), n.now(), uniqueInt())
}

func newL(drift, t, seq uint64) (uid UID) {
	d := (drift - 22) << 61
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
func (n Alloc) G(interval ...time.Duration) UID {
	return newG(n.uint64, drift(interval...), n.now(), uniqueInt())
}

func newG(n, drift, t, seq uint64) (uid UID) {
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
func drift(interval ...time.Duration) uint64 {
	switch {
	case len(interval) == 0:
		return 26
	case interval[0] <= 34*time.Second:
		return 29
	case interval[0] <= 68*time.Second:
		return 28
	case interval[0] <= 137*time.Second:
		return 27
	case interval[0] <= 274*time.Second:
		return 26
	case interval[0] <= 549*time.Second:
		return 25
	case interval[0] <= 1099*time.Second:
		return 24
	case interval[0] <= 2199*time.Second:
		return 23
	default:
		return 22
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
	dd := (drift - 22) << 29

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

LtoG casts local (64-bit) k-order UID to global (96-bit) one
*/
func (n Alloc) LtoG(l UID) (uid UID) {
	if l.hi != 0 {
		uid = l
		return
	}

	d := (l.lo >> 61) + 22
	return newG(n.uint64, d, uint64(l.Time()), l.Seq())
}

/*

GtoL casts global (96-bit) k-order value to local (64-bit) one
*/
func (n Alloc) GtoL(g UID) (uid UID) {
	if g.hi == 0 {
		uid = g
		return
	}

	d := (g.hi >> 29) + 22
	return newL(d, uint64(g.Time()), g.Seq())
}

/*

ID generates new k-order value and encodes it to string
*/
func (n Alloc) ID() string {
	return n.G().Chars()
}

/*

Time returns ⟨𝒕⟩ timestamp fraction from identifier.
The returned value is nano seconds compatible with time.Unix(0, uid.Time())
*/
func (uid UID) Time() int64 {
	// high bits are not defined for the local identifier
	if uid.hi == 0 {
		return int64(uid.lo << 3 >> (14 + 3) << (14 + 3))
	}

	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	d := (uid.hi >> 29) + 22
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
func (uid UID) Node() uint64 {
	// high bits are not defined for the local identifier
	if uid.hi == 0 {
		return 0
	}

	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	d := (uid.hi >> 29) + 22
	a := 64 - 14 - d
	b := 32 - a

	lo := uid.lo >> (d + 14)
	hi := uid.hi << (64 - b) >> (64 - b - a)

	return hi | lo
}

/*

Seq returns ⟨𝒔⟩ sequence value. The value of monotonic unique integer at the time of ID creation.
*/
func (uid UID) Seq() uint64 {
	return uid.lo & 0x3fff
}

/*

Eq compares k-order UIDs, returns true if values are equal
*/
func Eq(a, b UID) bool {
	return a.hi == b.hi && a.lo == b.lo
}

/*

Eq compares k-order UIDs, returns true if values are equal
Eq is an alias of `func Eq/2`
*/
func (uid UID) Eq(b UID) bool {
	return Eq(uid, b)
}

/*

Lt compares k-order UIDs, return true if value a less that value b
*/
func Lt(a, b UID) bool {
	if a.hi == 0 && b.hi == 0 {
		return a.lo < b.lo
	}

	if a.hi != 0 && b.hi != 0 {
		return a.hi <= b.hi && a.lo < b.lo
	}

	return false
}

/*

Lt compares k-order UIDs. It is an alias of `func Lt/2`
*/
func (uid UID) Lt(b UID) bool {
	return Lt(uid, b)
}

/*

Diff approximates distance between k-order UIDs.
*/
func Diff(a, b UID) UID {
	t := uint64(a.Time() - b.Time())
	s := a.Seq() - b.Seq()

	if a.hi == 0 {
		d := (a.lo >> 61) + 22
		return newL(d, t, s)
	}

	d := (a.hi >> 29) + 22
	return newG(a.Node(), d, t, s)
}

/*

Diff is an alias of `func Diff/2`
*/
func (uid UID) Diff(b UID) UID {
	return Diff(uid, b)
}

/*

Split decomposes UID value to bytes slice. The funcion acts as binary comprehension,
the value n defines number of bits to extract into each cell.
*/
func (uid UID) Split(n uint64) (bytes []byte) {
	size := uint64(96) // size of struct in bits
	hilo := uint64(64) // hi | lo division at
	if uid.hi == 0 {
		size = 64
	}
	bytes = make([]byte, size/n)

	mask := uint64(1<<n) - 1
	i := 0

	for a := size; a >= n; a -= n {
		b := a - n
		switch {
		case a >= hilo && b >= hilo:
			value := byte(uid.hi >> (b - hilo) & mask)
			bytes[i] = value
		case a <= hilo && b <= hilo:
			value := byte(uid.lo >> b & mask)
			bytes[i] = value
		case a > hilo && b < hilo:
			suffix := uint64(1<<(a-hilo)) - 1
			hi := byte(uid.hi & suffix)
			lo := byte(uid.lo >> b)
			bytes[i] = hi<<(hilo-b) | lo
		}
		i++
	}

	return
}

/*

Fold composes UID value from byte slice. The operation is inverse to Split.
*/
func (uid *UID) Fold(n uint64, bytes []byte) {
	size := uint64(96)
	hilo := uint64(64)

	mask := uint64(1<<n) - 1
	i := 0

	for a := size; a >= n; a -= n {
		b := a - n
		switch {
		case a >= hilo && b >= hilo:
			uid.hi |= (uint64(bytes[i]) & mask) << (b - hilo)
		case a <= hilo && b <= hilo:
			uid.lo |= (uint64(bytes[i]) & mask) << b
		case a > hilo && b < hilo:
			uid.hi |= (uint64(bytes[i]) & mask) >> (hilo - b)
			uid.lo |= (uint64(bytes[i]) & mask) << b
		}
		i++
	}
}

/*

Bytes encodes k-odered value to byte slice
*/
func (uid UID) Bytes() []byte {
	return uid.Split(8)
}

/*

Chars encodes k-ordered value to lexicographically sortable strings
*/
func (uid UID) Chars() string {
	return encode64(uid)
}

/*

UnmarshalJSON decodes lexicographically sortable strings to UID value
*/
func (uid *UID) UnmarshalJSON(b []byte) (err error) {
	var val string
	if err = json.Unmarshal(b, &val); err != nil {
		return
	}
	*uid = Decode(val)
	return
}

/*

MarshalJSON encodes k-ordered value to lexicographically sortable JSON strings
*/
func (uid UID) MarshalJSON() (bytes []byte, err error) {
	val := Encode(uid)
	return json.Marshal(val)
}

/*

Encode converts k-order UID to lexicographically sortable strings
*/
func Encode(uid UID) string {
	return encode64(uid)
}

/*

Decode converts k-order UID from lexicographically sortable strings
*/
func Decode(uid string) UID {
	return decode64(uid)
}

/*

EncodeBytes converts k-order UID to lexicographically sortable binary
*/
func EncodeBytes(uid UID) []byte {
	return uid.Split(8)
}

/*

DecodeBytes converts k-order UID from lexicographically sortable binary
*/
func DecodeBytes(val []byte) (uid UID) {
	uid.Fold(8, val)
	return
}
