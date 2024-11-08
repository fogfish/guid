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
	"unsafe"
)

// K is native representation of k-ordered number.
// The structure is dedicated for both local and global k-ordered values.
// The local k-ordered value do not uses Hi fraction (equal to 0).
// The global k-ordered value is 96-bit long and requires no central registration process.
//
// Note: Golang struct is 128-bits but only 96-bits are used effectively.
// The serialization process ensures that only 96-bits are used.
type K struct{ Hi, Lo uint64 }

// UnmarshalJSON decodes lexicographically sortable strings to UID value
func (uid *K) UnmarshalJSON(b []byte) (err error) {
	var val string
	if err = json.Unmarshal(b, &val); err != nil {
		return
	}

	if val[0] == '*' {
		*uid, err = FromStringG(val[1:])
		if err != nil {
			return err
		}

		*uid = ToL(*uid)
		return nil
	}

	*uid, err = FromStringG(val)
	return err
}

// MarshalJSON encodes k-ordered value to lexicographically sortable JSON strings
func (uid K) MarshalJSON() (bytes []byte, err error) {
	if uid.Hi == 0 {
		return json.Marshal("*" + String(FromL(Clock, uid)))
	}

	return json.Marshal(String(uid))
}

// String encoding of K-Order value
func (uid K) String() string {
	return String(uid)
}

const (
	bitsDrift    = 3
	bitsSeq      = 14
	bitsSeqDrift = bitsSeq + bitsDrift
	bytesInG     = 12
	bytesInL     = 8
)

// Z returns "zero" local (64-bit) k-order identifier
func Z(clock Chronos, drift ...time.Duration) (uid K) {
	t, seq := uint64(0), uint64(0)
	return makeG(0, driftInBits(drift), t, seq)
}

// Generates globally unique 96-bit k-ordered identifier.
//
//	3bit  47 bit - 𝒅 bit         32 bit     𝒅 bit  14 bit
//	|-|-------------------|----------------|-----|-------|
//	⟨𝒅⟩        ⟨𝒕⟩                ⟨𝒍⟩         ⟨𝒕⟩     ⟨𝒔⟩
func G(clock Chronos, drift ...time.Duration) K {
	t, seq := clock.T()
	return makeG(clock.L(), driftInBits(drift), t, seq)
}

func makeG(n, drift, t, seq uint64) (uid K) {
	thi, tlo := splitT(t, drift)
	nhi, nlo := splitNode(n, drift)

	// Note: with drift = 30 sec, nhi = 0
	uid.Hi = thi | nhi
	uid.Lo = nlo | tlo | seq

	return
}

// Generates locally unique 64-bit k-order identifier.
//
// 3bit        47 bit           14 bit
// |-|------------------------|-------|
// ⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩

func L(clock Chronos, drift ...time.Duration) K {
	t, seq := clock.T()
	return makeL(driftInBits(drift), t, seq)
}

func makeL(drift, t, seq uint64) (uid K) {
	d := (drift - driftZ) << 61
	x := t >> bitsSeqDrift << bitsSeq

	uid.Hi = 0
	uid.Lo = d | x | seq

	return
}

// Equal compares k-order UIDs, returns true if values are equal
func Equal(a, b K) bool {
	return a.Hi == b.Hi && a.Lo == b.Lo
}

// Before checks if k-ordered value A is before value B
func Before(a, b K) bool {
	return (a.Hi < b.Hi) || (a.Hi == b.Hi && a.Lo < b.Lo)
}

// After checks if k-ordered value A is after value B
func After(a, b K) bool {
	return (a.Hi > b.Hi) || (a.Hi == b.Hi && a.Lo > b.Lo)
}

// Time returns ⟨𝒕⟩ timestamp fraction from identifier in nano seconds
func Time(uid K) uint64 {
	if uid.Hi == 0 {
		return timeL(uid)

	}
	return timeG(uid)
}

func timeG(uid K) uint64 {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	d := (uid.Hi >> 29) + driftZ
	a := 64 - bitsSeq - d
	b := 32 - a

	hi := (uid.Hi >> b) << d
	lo := (uid.Lo << a) >> (64 - d)

	t := ((hi | lo) << bitsSeqDrift)
	return t
}

func timeL(uid K) uint64 {
	return uint64(uid.Lo) << 3 >> bitsSeqDrift << bitsSeqDrift
}

// EpochT convers ⟨𝒕⟩ timestamp fraction from identifier as unix timestamp
func EpochT(uid K) time.Time {
	return time.Unix(0, int64(Time(uid)))
}

// EpochI (inverse) convers ⟨𝒕⟩ timestamp fraction from identifier as unix timestamp
func EpochI(uid K) time.Time {
	t := 0xffffffffffffffff - Time(uid)
	return time.Unix(0, int64(t))
}

// Node returns ⟨𝒍⟩ location fraction from identifier.
func Node(uid K) uint64 {
	if uid.Hi == 0 {
		return 0
	}

	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	d := (uid.Hi >> 29) + driftZ
	a := 64 - bitsSeq - d
	b := 32 - a

	lo := uid.Lo >> (d + bitsSeq)
	hi := uid.Hi << (64 - b) >> (64 - b - a)

	return hi | lo
}

// Seq returns ⟨𝒔⟩ sequence value. The value of monotonic unique integer
// at the time of K-ordered value creation.
func Seq(uid K) uint64 {
	return uid.Lo & 0x3fff
}

// Diff approximates distance between k-order UIDs.
func Diff(a, b K) K {
	t := Time(a) - Time(b)
	s := Seq(a) - Seq(b)

	if a.Hi != 0 && b.Hi != 0 {
		d := (a.Hi >> 29) + driftZ
		return makeG(Node(a), d, t, s)
	}

	d := (uint64(a.Lo) >> 61) + driftZ
	return makeL(d, t, s)
}

// Casts local (64-bit) k-order UID to global (96-bit) one
func FromL(clock Chronos, uid K) K {
	if uid.Hi != 0 {
		return uid
	}

	d := (uint64(uid.Lo) >> 61) + driftZ
	return makeG(clock.L(), d, Time(uid), Seq(uid))
}

// Casts global (96-bit) k-order value to local (64-bit) one
func ToL(uid K) K {
	if uid.Hi == 0 {
		return uid
	}

	d := (uid.Hi >> 29) + driftZ
	return makeL(d, Time(uid), Seq(uid))
}

// FromT converts unix timestamp to local K-order value
func FromT(t time.Time, drift ...time.Duration) K {
	return makeL(driftInBits(drift), uint64(t.UnixNano()), 0)
}

// Split decomposes UID value to bytes slice. The function acts as binary comprehension,
// the value n defines number of bits to extract into each cell.
func Split(n uint64, uid K) (bytes []byte) {
	if uid.Hi == 0 {
		b := make([]byte, 64/n)
		split(0, uint64(uid.Lo), 64, n, b)
		return b
	}

	b := make([]byte, 96/n)
	split(uid.Hi, uid.Lo, 96, n, b)
	return b
}

// Fold composes UID value from byte slice. The operation is inverse to Split.
func FoldG(n uint64, bytes []byte) (uid K) {
	uid.Hi, uid.Lo = fold(96, n, bytes)
	return
}

// Fold composes UID value from byte slice. The operation is inverse to Split.
func FoldL(n uint64, bytes []byte) (uid K) {
	uid.Hi, uid.Lo = fold(64, n, bytes)
	return
}

// Bytes encodes k-odered value to byte slice
func Bytes(uid K) []byte {
	if uid.Hi == 0 {
		var (
			buf [8]byte
			bfs = buf[:]
		)
		split(0, uint64(uid.Lo), 64, 8, bfs)
		return bfs
	}

	var (
		buf [12]byte
		bfs = buf[:]
	)

	split(uid.Hi, uid.Lo, 96, 8, bfs)
	return bfs
}

// FromBytes decodes converts k-order UID from bytes
func FromBytes(val []byte) (K, error) {
	switch len(val) {
	case bytesInG:
		return FoldG(8, val), nil
	case bytesInL:
		return FoldL(8, val), nil
	default:
		return K{}, fmt.Errorf("malformed k-order number: %v", val)
	}
}

// Encodes k-ordered value to lexicographically sortable base62 strings
func Base62(uid K) string {
	str := encode62(Bytes(uid))
	return *(*string)(unsafe.Pointer(&str))
}

// FromBase62 decodes converts k-order UID from base62 string
func FromBase62(val string) (K, error) {
	b, err := decode62([]byte(val))
	if err != nil {
		return K{}, err
	}

	return FromBytes(b)
}

// String encodes k-ordered value to lexicographically sortable strings
func String(uid K) string {
	var (
		buf [16]byte // interim buffer where uid is split as seq of bytes
		enc [16]byte // output encoded string
		bfs = buf[:]
	)

	if uid.Hi == 0 {
		split(0, uid.Lo, 64, 4, bfs)
	} else {
		split(uid.Hi, uid.Lo, 96, 6, bfs)
	}

	encode64(buf, &enc)
	str := enc[:]
	return *(*string)(unsafe.Pointer(&str))
}

// FromStringG decodes converts k-order UID from lexicographically sortable strings
func FromStringG(val string) (K, error) {
	if len(val) != 16 {
		return K{}, fmt.Errorf("malformed k-order number: %v", val)
	}

	return FoldG(6, decode64(val)), nil
}

// FromStringL decodes converts k-order UID from lexicographically sortable strings
func FromStringL(val string) (K, error) {
	if len(val) != 16 {
		return K{}, fmt.Errorf("malformed k-order number: %v", val)
	}

	return FoldL(4, decode64(val)), nil
}
