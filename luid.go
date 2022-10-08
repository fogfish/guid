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

// LUID is namespace to manipulate local k-order identity
type LUID string

// L instance of local k-order identity
const L LUID = "guid.L"

/*

Z returns "zero" local (64-bit) k-order identifier
*/
func (LUID) Z(clock Chronos, drift ...time.Duration) LID {
	// all bits are 0 in "zero" unique 64-bit k-order identifier.
	// but it requites to that 3bit of ⟨𝒅⟩ is set
	d := (driftInBits(drift) - driftZ) << 61
	return LID(d)
}

/*

New generates locally unique 64-bit k-order identifier.

  3bit        47 bit           14 bit
  |-|------------------------|-------|
  ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩

*/
func (LUID) K(clock Chronos, drift ...time.Duration) LID {
	t, seq := clock.T()
	return mkLUID(driftInBits(drift), t, seq)
}

func mkLUID(drift, t, seq uint64) LID {
	d := (drift - driftZ) << 61
	x := t >> (14 + 3) << 14

	return LID(d | x | seq)
}

/*

Eq compares k-order UIDs, returns true if values are equal
*/
func (LUID) Equal(a, b LID) bool {
	return a == b
}

/*

Lt compares k-order UIDs, return true if value uid (this) less
than value b (argument)
*/
func (LUID) Less(a, b LID) bool {
	return a < b
}

/*

Time returns ⟨𝒕⟩ timestamp fraction from identifier.
The returned value is nano seconds compatible with time.Unix(0, uid.Time())
*/
func (LUID) Time(uid LID) uint64 {
	return uint64(uid) << 3 >> (14 + 3) << (14 + 3)
}

/*

EpochT convers ⟨𝒕⟩ timestamp fraction from identifier as unix timestamp
*/
func (ns LUID) EpochT(uid LID) time.Time {
	return time.Unix(0, int64(ns.Time(uid)))
}

/*

EpochI (inverse) convers ⟨𝒕⟩ timestamp fraction from identifier as unix timestamp
*/
func (ns LUID) EpochI(uid LID) time.Time {
	t := 0xffffffffffffffff - ns.Time(uid)
	return time.Unix(0, int64(t))
}

/*

Seq returns ⟨𝒔⟩ sequence value. The value of monotonic unique integer
at the time of ID creation.
*/
func (LUID) Seq(uid LID) uint64 {
	return uint64(uid) & 0x3fff
}

/*

Diff approximates distance between k-order UIDs.
*/
func (ns LUID) Diff(a, b LID) LID {
	t := ns.Time(a) - ns.Time(b)
	s := ns.Seq(a) - ns.Seq(b)
	d := (uint64(a) >> 61) + driftZ
	return mkLUID(d, t, s)
}

/*

Split decomposes UID value to bytes slice. The funcion acts as binary comprehension,
the value n defines number of bits to extract into each cell.
*/
// LID & GID LSplit GSplit
func (LUID) Split(n uint64, uid LID) (bytes []byte) {
	return split(0, uint64(uid), 64, n)
}

/*

Fold composes UID value from byte slice. The operation is inverse to Split.
*/
func (LUID) Fold(n uint64, bytes []byte) LID {
	_, lo := fold(64, n, bytes)
	return LID(lo)
}

/*

FromT converts unix timestamp to local K-order value
*/
// LID & GID LFromBytes GFromBytes
func (LUID) FromT(t time.Time, drift ...time.Duration) LID {
	return mkLUID(driftInBits(drift), uint64(t.UnixNano()), 0)
}

/*

FromG casts global (96-bit) k-order value to local (64-bit) one
*/
func (LUID) FromG(uid GID) LID {
	d := (uid.Hi >> 29) + driftZ
	return mkLUID(d, G.Time(uid), G.Seq(uid))
}

/*

FromBytes decodes converts k-order UID from bytes
*/
func (ns LUID) FromBytes(val []byte) LID {
	if len(val) != 8 {
		panic(fmt.Errorf("malformed local k-order number: %v", val))
	}

	return ns.Fold(8, val)
}

/*

FromString decodes converts k-order UID from lexicographically sortable strings
*/
func (ns LUID) FromString(val string) LID {
	return ns.Fold(4, decode64(val))
}

/*

Bytes encodes k-odered value to byte slice
*/
func (ns LUID) Bytes(uid LID) []byte {
	return ns.Split(8, uid)
}

/*

String encodes k-ordered value to lexicographically sortable strings
*/
func (ns LUID) String(uid LID) string {
	return encode64(ns.Split(4, uid))
}

/*******************************************************************************

GID

*******************************************************************************/

/*

UnmarshalJSON decodes lexicographically sortable strings to UID value
*/
func (uid *LID) UnmarshalJSON(b []byte) (err error) {
	var val string
	if err = json.Unmarshal(b, &val); err != nil {
		return
	}
	*uid = L.FromString(val)
	return
}

/*

MarshalJSON encodes k-ordered value to lexicographically sortable JSON strings
*/
func (uid LID) MarshalJSON() (bytes []byte, err error) {
	return json.Marshal(L.String(uid))
}

/*

String encoding of K-Order value
*/
func (uid LID) String() string {
	return L.String(uid)
}

/*

type X interface {
	Z
	K

	Eq
	Lt

	Diff
	Split
	Fold


	ToG
	ToL


	FromBytes
	FromString
	FromTime
}


guid.G.K()

guid.L.xxx
guid.G.xxx

*/

/*******************************************************************************

LID

*******************************************************************************/

/*

Time returns ⟨𝒕⟩ timestamp fraction from identifier.
The returned value is nano seconds compatible with time.Unix(0, uid.Time())
*/
func (uid LID) Time() uint64 {
	return uint64(uid) << 3 >> (14 + 3) << (14 + 3)
}
