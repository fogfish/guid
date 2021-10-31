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

package guid

import (
	"encoding/json"
	"fmt"
	"time"
)

// zero point for drift drift
const driftZ = 18

/*

Z returns "zero" local (64-bit) k-order identifier
*/
func Z(clock Chronos, drift ...time.Duration) (uid K) {
	// all bits are 0 in "zero" unique 64-bit k-order identifier.
	// but it requites to that 3bit of ⟨𝒅⟩ is set
	d := (driftInBits(drift) - driftZ) << 61
	uid.lo = d
	return
}

/*

L generates locally unique 64-bit k-order identifier.

  3bit        47 bit           14 bit
  |-|------------------------|-------|
  ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩

*/
func L(clock Chronos, drift ...time.Duration) K {
	t, seq := clock.T()
	return mkLUID(driftInBits(drift), t, seq)
}

func mkLUID(drift, t, seq uint64) (uid K) {
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
func G(clock Chronos, drift ...time.Duration) K {
	t, seq := clock.T()
	return mkGUID(clock.L(), driftInBits(drift), t, seq)
}

func mkGUID(n, drift, t, seq uint64) (uid K) {
	thi, tlo := splitT(t, drift)
	nhi, nlo := splitNode(n, drift)

	// Note: with drift = 30 sec, nhi = 0
	uid.hi = thi | nhi
	uid.lo = nlo | tlo | seq

	return
}

/*

driftBits converts a time drift into number of bits to shift the location
fraction. E.g. if application allows 2 min time drift in the system than last
20 bits of timestamp becomes less significant than location.

The default drift is approximately 5 min, the drift value is encoded as
3 bits, which gives 8 possible values
*/
func driftInBits(drift []time.Duration) uint64 {
	switch {
	case len(drift) == 0:
		return driftZ + 3
	case drift[0] <= 34*time.Second:
		return driftZ
	case drift[0] <= 68*time.Second:
		return driftZ + 1
	case drift[0] <= 137*time.Second:
		return driftZ + 2
	case drift[0] <= 274*time.Second:
		return driftZ + 3
	case drift[0] <= 549*time.Second:
		return driftZ + 4
	case drift[0] <= 1099*time.Second:
		return driftZ + 5
	case drift[0] <= 2199*time.Second:
		return driftZ + 6
	default:
		return driftZ + 7
	}
}

// splits ⟨𝒕⟩ faction (timestamp) to hi and lo bits of K order value
func splitT(t uint64, drift uint64) (uint64, uint64) {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	// 14 bits of time is exchange for seq
	//  3 bits is reserved for drift
	//    initial timestamp is reduced by 17 bits ~ 10⁶ nanoseconds
	x := t >> (14 + 3)
	a := 64 - 14 - drift
	b := 32 - a

	lo := (x << (a + 14)) >> a
	hi := (x >> drift) << b
	dd := (drift - driftZ) << 29

	return hi | dd, lo
}

// split ⟨𝒍⟩ faction (location) to hi and lo bits of K order value
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

/*******************************************************************************

Lenses of K-Order Number

*******************************************************************************/

/*

Time returns ⟨𝒕⟩ timestamp fraction from identifier.
The returned value is nano seconds compatible with time.Unix(0, uid.Time())
*/
func Time(uid K) uint64 {
	if uid.hi == 0 {
		return localT(uid)
	}

	return globalT(uid)
}

/*

Time returns ⟨𝒕⟩ timestamp fraction from local identifier.
*/
func localT(uid K) uint64 {
	return uid.lo << 3 >> (14 + 3) << (14 + 3)
}

/*

Time returns ⟨𝒕⟩ timestamp fraction from identifier.
The returned value is nano seconds compatible with time.Unix(0, uid.Time())
*/
func globalT(uid K) uint64 {
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
	return t //& 0x7fffffffffffffff
}

/*

Node returns ⟨𝒍⟩ location fraction from identifier.
*/
func Node(uid K) uint64 {
	if uid.hi == 0 {
		return 0
	}

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

Seq returns ⟨𝒔⟩ sequence value. The value of monotonic unique integer
at the time of ID creation.
*/
func Seq(uid K) uint64 {
	return uid.lo & 0x3fff
}

/*******************************************************************************

K-Order "Algebra"

*******************************************************************************/

/*

ToG casts local (64-bit) k-order UID to global (96-bit) one
*/
func ToG(clock Chronos, uid K) K {
	if uid.hi != 0 {
		return uid
	}

	d := (uid.lo >> 61) + driftZ
	return mkGUID(clock.L(), d, localT(uid), Seq(uid))
}

/*

ToL casts global (96-bit) k-order value to local (64-bit) one
*/
func ToL(uid K) K {
	if uid.hi == 0 {
		return uid
	}

	d := (uid.hi >> 29) + driftZ
	return mkLUID(d, globalT(uid), Seq(uid))
}

/*

Eq compares k-order UIDs, returns true if values are equal
*/
func Eq(a, b K) bool {
	return a.hi == b.hi && a.lo == b.lo
}

/*

Lt compares k-order UIDs, return true if value uid (this) less
than value b (argument)
*/
func Lt(a, b K) bool {
	return a.hi <= b.hi && a.lo < b.lo
}

/*

Diff approximates distance between k-order UIDs.
*/
func Diff(a, b K) K {
	if a.hi == 0 && b.hi == 0 {
		return diffL(a, b)
	}

	return diffG(a, b)
}

func diffL(a, b K) K {
	t := localT(a) - localT(b)
	s := Seq(a) - Seq(b)
	d := (a.lo >> 61) + driftZ
	return mkLUID(d, t, s)
}

/*

Diff approximates distance between k-order UIDs.
*/
func diffG(a, b K) K {
	t := globalT(a) - globalT(b)
	s := Seq(a) - Seq(b)
	d := (a.hi >> 29) + driftZ
	return mkGUID(Node(a), d, t, s)
}

/*

Split decomposes UID value to bytes slice. The funcion acts as binary comprehension,
the value n defines number of bits to extract into each cell.
*/
func Split(uid K, n uint64) (bytes []byte) {
	if uid.hi == 0 {
		return split(0, uid.lo, 64, n)
	}

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

LFold composes UID value from byte slice. The operation is inverse to Split.
*/
func LFold(n uint64, bytes []byte) (uid K) {
	_, uid.lo = fold(64, n, bytes)
	return
}

/*

GFold composes UID value from byte slice. The operation is inverse to Split.
*/
func GFold(n uint64, bytes []byte) (uid K) {
	uid.hi, uid.lo = fold(96, n, bytes)
	return
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
func Bytes(uid K) []byte {
	return Split(uid, 8)
}

/*

String encodes k-ordered value to lexicographically sortable strings
*/
func String(uid K) string {
	// Note: encoding local and global values to string produces result of
	//       same length. It is not possible to distinguish local from global
	//       using string encoding. Thus both are encoded as 96 bit binaries
	return encode64(split(uid.hi, uid.lo, 96, 6))
}

/*

FromBytes decodes converts k-order UID from bytes
*/
func FromBytes(val []byte) K {
	switch len(val) {
	case 8:
		return LFold(8, val)
	case 12:
		return GFold(8, val)
	default:
		panic(fmt.Errorf("malformed k-order number: %v", val))
	}
}

/*

FromString decodes converts k-order UID from lexicographically sortable strings
*/
func FromString(val string) K {
	// Note: encoding local and global values to string produces result of
	//       same length. It is not possible to distinguish local from global
	//       using string encoding. Thus both are encoded as 96 bit binaries
	return GFold(6, decode64(val))
}

/*

FromTime converts unix timestamp to new local K-order value
*/
func FromTime(t time.Time, drift ...time.Duration) K {
	return mkLUID(driftInBits(drift), uint64(t.UnixNano()), 0)
}

/*

UnmarshalJSON decodes lexicographically sortable strings to UID value
*/
func (uid *K) UnmarshalJSON(b []byte) (err error) {
	var val string
	if err = json.Unmarshal(b, &val); err != nil {
		return
	}
	*uid = FromString(val)
	return
}

/*

MarshalJSON encodes k-ordered value to lexicographically sortable JSON strings
*/
func (uid K) MarshalJSON() (bytes []byte, err error) {
	return json.Marshal(String(uid))
}

/*

String encoding of K-Order value
*/
func (uid K) String() string {
	return String(uid)
}
