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

// GUID is namespace to manupulate local k-order identity
type GUID string

// G instance of local k-order identity
const G GUID = "guid.G"

/*

Z returns "zero" local (64-bit) k-order identifier
*/
func (GUID) Z(clock Chronos, drift ...time.Duration) (uid GID) {
	// TODO:
	// all bits are 0 in "zero" unique 64-bit k-order identifier.
	// but it requites to that 3bit of ⟨𝒅⟩ is set
	d := (driftInBits(drift) - driftZ) << 61
	uid.lo = d
	return
}

/*

K generate globally unique 96-bit k-order identifier.

  3bit  47 bit - 𝒅 bit         32 bit     𝒅 bit  14 bit
  |-|-------------------|----------------|-----|-------|
  ⟨𝒅⟩        ⟨𝒕⟩                ⟨𝒍⟩         ⟨𝒕⟩     ⟨𝒔⟩

*/
func (GUID) K(clock Chronos, drift ...time.Duration) GID {
	t, seq := clock.T()
	return mkGUID(clock.L(), driftInBits(drift), t, seq)
}

func mkGUID(n, drift, t, seq uint64) (uid GID) {
	thi, tlo := splitT(t, drift)
	nhi, nlo := splitNode(n, drift)

	// Note: with drift = 30 sec, nhi = 0
	uid.hi = thi | nhi
	uid.lo = nlo | tlo | seq

	return
}

/*

Eq compares k-order UIDs, returns true if values are equal
*/
func (GUID) Eq(a, b GID) bool {
	return a.hi == b.hi && a.lo == b.lo
}

/*

Lt compares k-order UIDs, return true if value uid (this) less
than value b (argument)
*/
func (GUID) Lt(a, b GID) bool {
	return a.hi <= b.hi && a.lo < b.lo
}

/*

Time returns ⟨𝒕⟩ timestamp fraction from identifier.
The returned value is nano seconds compatible with time.Unix(0, uid.Time())
*/
func (GUID) Time(uid GID) uint64 {
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

EpochT convers ⟨𝒕⟩ timestamp fraction from identifier as unix timestamp
*/
func (ns GUID) EpochT(uid GID) time.Time {
	return time.Unix(0, int64(ns.Time(uid)))
}

/*

EpochI (inverse) convers ⟨𝒕⟩ timestamp fraction from identifier as unix timestamp
*/
func (ns GUID) EpochI(uid GID) time.Time {
	t := 0xffffffffffffffff - ns.Time(uid)
	return time.Unix(0, int64(t))
}

/*

Node returns ⟨𝒍⟩ location fraction from identifier.
*/
func (GUID) Node(uid GID) uint64 {
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
func (GUID) Seq(uid GID) uint64 {
	return uid.lo & 0x3fff
}

/*

Diff approximates distance between k-order UIDs.
*/
func (ns GUID) Diff(a, b GID) GID {
	t := ns.Time(a) - ns.Time(b)
	s := ns.Seq(a) - ns.Seq(b)
	d := (a.hi >> 29) + driftZ
	return mkGUID(ns.Node(a), d, t, s)
}

/*

Split decomposes UID value to bytes slice. The funcion acts as binary comprehension,
the value n defines number of bits to extract into each cell.
*/
func (GUID) Split(n uint64, uid GID) (bytes []byte) {
	return split(uid.hi, uid.lo, 96, n)
}

/*

Fold composes UID value from byte slice. The operation is inverse to Split.
*/
func (GUID) Fold(n uint64, bytes []byte) (uid GID) {
	uid.hi, uid.lo = fold(96, n, bytes)
	return
}

/*

FromL casts local (64-bit) k-order UID to global (96-bit) one
*/
func (ns GUID) FromL(clock Chronos, uid LID) GID {
	d := (uid.lo >> 61) + driftZ
	return mkGUID(clock.L(), d, L.Time(uid), L.Seq(uid))
}

/*

FromBytes decodes converts k-order UID from bytes
*/
func (ns GUID) FromBytes(val []byte) GID {
	if len(val) != 12 {
		panic(fmt.Errorf("malformed global k-order number: %v", val))
	}

	return ns.Fold(8, val)
}

/*

FromString decodes converts k-order UID from lexicographically sortable strings
*/
// LID & GID LFromBytes GFromBytes
func (ns GUID) FromString(val string) GID {
	return ns.Fold(6, decode64(val))
}

/*

Bytes encodes k-odered value to byte slice
*/
func (ns GUID) Bytes(uid GID) []byte {
	return ns.Split(8, uid)
}

/*

String encodes k-ordered value to lexicographically sortable strings
*/
func (ns GUID) String(uid GID) string {
	return encode64(ns.Split(6, uid))
}

/*******************************************************************************

GID

*******************************************************************************/

/*

UnmarshalJSON decodes lexicographically sortable strings to UID value
*/
func (uid *GID) UnmarshalJSON(b []byte) (err error) {
	var val string
	if err = json.Unmarshal(b, &val); err != nil {
		return
	}
	*uid = G.FromString(val)
	return
}

/*

MarshalJSON encodes k-ordered value to lexicographically sortable JSON strings
*/
func (uid GID) MarshalJSON() (bytes []byte, err error) {
	return json.Marshal(G.String(uid))
}

/*

String encoding of K-Order value
*/
func (uid GID) String() string {
	return G.String(uid)
}
