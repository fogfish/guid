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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"
	"unsafe"
)

// G is a globally unique 96-bit k-ordered value. It requires no central
// registration process.
//
//	3bit  47 bit - 𝒅 bit         32 bit     𝒅 bit  14 bit
//	|-|-------------------|----------------|-----|-------|
//	⟨𝒅⟩        ⟨𝒕⟩                ⟨𝒍⟩         ⟨𝒕⟩     ⟨𝒔⟩
//
// The type holds the big-endian representation of that 96-bit number and
// nothing else, which makes it exactly 12 bytes wide with single byte
// alignment. The representation is the one the schema is defined in, so
// memory I/O costs nothing:
//
//	↣ a [n]G or []G packs without padding, 12 bytes per value;
//
//	↣ the wire format is the value itself, g[:] is a valid encoding and
//	  copy(g[:], buf) a valid decoding, neither shifts a single bit;
//
//	↣ the byte order is the order of the identifiers, so bytes.Compare,
//	  bytes.Equal and sort.Slice over the raw bytes agree with Before,
//	  After and Equal.
type G [SizeG]byte

// NewG allocates globally unique 96-bit k-ordered value.
//
// The ⟨𝒅⟩ drift is taken from the clock, see WithDrift, so that every value of
// a keyspace is allocated with the same one.
func NewG(clock Chronos) G {
	t, seq := clock.T()
	return makeG(clock.Node(), clock.Drift(), t, seq)
}

// ZeroG returns the "zero" 96-bit k-ordered value, the value that precedes
// every value allocated by the clock.
func ZeroG(clock Chronos) G {
	return makeG(0, clock.Drift(), 0, 0)
}

func makeG(n, drift, t, seq uint64) G {
	thi, tlo := splitT(t, drift)
	nhi, nlo := splitNode(n, drift)

	// Note: with drift = 30 sec, nhi = 0
	return joinG(thi|nhi, nlo|tlo|seq)
}

// words decomposes the value into the pair (hi, lo) so that the value equals
// hi·2⁶⁴ + lo. Only the low 32 bits of hi are in use.
func (uid G) words() (uint64, uint64) {
	return uint64(binary.BigEndian.Uint32(uid[0:4])), binary.BigEndian.Uint64(uid[4:12])
}

// joinG is the inverse of words
func joinG(hi, lo uint64) (uid G) {
	binary.BigEndian.PutUint32(uid[0:4], uint32(hi))
	binary.BigEndian.PutUint64(uid[4:12], lo)
	return
}

// drift returns the ⟨𝒅⟩ fraction as the number of bits of ⟨𝒕⟩ that rank below
// ⟨𝒍⟩. The code occupies the 3 most significant bits of the value.
func (uid G) drift() uint64 { return uint64(uid[0]>>5) + driftZ }

// Equal compares k-ordered values, returns true if values are equal
func (uid G) Equal(b G) bool { return uid == b }

// Before checks if k-ordered value A is before value B
func (uid G) Before(b G) bool {
	ahi, alo := uid.words()
	bhi, blo := b.words()
	return ahi < bhi || (ahi == bhi && alo < blo)
}

// After checks if k-ordered value A is after value B
func (uid G) After(b G) bool {
	ahi, alo := uid.words()
	bhi, blo := b.words()
	return ahi > bhi || (ahi == bhi && alo > blo)
}

// Time returns ⟨𝒕⟩ timestamp fraction from identifier in nano seconds
func (uid G) Time() uint64 {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	xhi, xlo := uid.words()
	d := uid.drift()
	a := 64 - bitsSeq - d
	b := 32 - a

	hi := (xhi >> b) << d
	lo := (xlo << a) >> (64 - d)

	return (hi | lo) << bitsSeqDrift
}

// Node returns ⟨𝒍⟩ location fraction from identifier.
func (uid G) Node() uint64 {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	xhi, xlo := uid.words()
	d := uid.drift()
	a := 64 - bitsSeq - d
	b := 32 - a

	hi := xhi << (64 - b) >> (64 - b - a)
	lo := xlo >> (d + bitsSeq)

	return hi | lo
}

// Seq returns ⟨𝒔⟩ sequence value. The value of monotonic unique integer
// at the time of k-ordered value creation.
func (uid G) Seq() uint64 {
	return uint64(binary.BigEndian.Uint16(uid[10:12]) & maskSeq)
}

// Epoch returns the wall clock instant the value was allocated at.
//
// The instant is a fact about the allocation, not about the layout of the
// keyspace: a value allocated by a descending clock reports the same time as
// one allocated by a forward clock at the same moment, see TimeOrder. Ordering
// questions are answered by Before, After and Time instead.
//
// The instant is accurate to one tick, 2¹⁷ ns ≈ 131 µs, and is meaningful only
// for a clock whose ⟨𝒕⟩ is unix nanoseconds, which is every clock but one built
// on a custom generator in other units, see WithClock.
func (uid G) Epoch() time.Time {
	return epoch(uid.Time())
}

// Diff approximates distance between k-ordered values.
func (uid G) Diff(b G) G {
	return makeG(uid.Node(), uid.drift(), uid.Time()-b.Time(), uid.Seq()-b.Seq())
}

// ToL casts globally unique 96-bit value to locally unique 64-bit one by
// dropping the ⟨𝒍⟩ fraction.
func (uid G) ToL() L {
	return makeL(uid.drift(), uid.Time(), uid.Seq())
}

// Bytes encodes k-ordered value to byte slice.
//
// The value is already its own encoding, uid[:] is the same bytes without the
// copy. Use it on the hot path when the slice does not outlive the value.
func (uid G) Bytes() []byte {
	b := make([]byte, SizeG)
	copy(b, uid[:])
	return b
}

// Split decomposes the value to bytes slice. The function acts as binary
// comprehension, the value n defines number of bits to extract into each cell.
func (uid G) Split(n uint64) []byte {
	hi, lo := uid.words()
	b := make([]byte, SizeG*8/n)
	split(hi, lo, SizeG*8, n, b)
	return b
}

// String encodes k-ordered value to lexicographically sortable string
func (uid G) String() string {
	var (
		buf [SizeString]byte // interim buffer where uid is split as seq of bytes
		enc [SizeString]byte // output encoded string
	)

	hi, lo := uid.words()
	split(hi, lo, SizeG*8, 6, buf[:])

	encode64(buf, &enc)
	str := enc[:]
	return *(*string)(unsafe.Pointer(&str))
}

// Base62 encodes k-ordered value to lexicographically sortable base62 string
func (uid G) Base62() string {
	str := encode62(uid[:])
	return *(*string)(unsafe.Pointer(&str))
}

// MarshalJSON encodes k-ordered value to lexicographically sortable JSON string
func (uid G) MarshalJSON() ([]byte, error) {
	return json.Marshal(uid.String())
}

// UnmarshalJSON decodes lexicographically sortable string to k-ordered value
func (uid *G) UnmarshalJSON(b []byte) error {
	var val string
	if err := json.Unmarshal(b, &val); err != nil {
		return err
	}

	v, err := FromStringG(val)
	if err != nil {
		return err
	}

	*uid = v
	return nil
}

// FoldG composes k-ordered value from byte slice. The operation is inverse
// to Split.
func FoldG(n uint64, bytes []byte) G {
	hi, lo := fold(SizeG*8, n, bytes)
	return joinG(hi, lo)
}

// FromBytesG decodes k-ordered value from bytes
func FromBytesG(val []byte) (G, error) {
	if len(val) != SizeG {
		return G{}, fmt.Errorf("malformed k-order number: %v", val)
	}

	var uid G
	copy(uid[:], val)
	return uid, nil
}

// FromStringG decodes k-ordered value from lexicographically sortable string
func FromStringG(val string) (G, error) {
	if len(val) != SizeString {
		return G{}, fmt.Errorf("malformed k-order number: %v", val)
	}

	return FoldG(6, decode64(val)), nil
}

// FromBase62G decodes k-ordered value from base62 string
func FromBase62G(val string) (G, error) {
	b, err := decode62([]byte(val))
	if err != nil {
		return G{}, err
	}

	// base62 is a positional numeral system, it does not carry leading zeros
	if len(b) > SizeG {
		return G{}, fmt.Errorf("malformed k-order number: %v", val)
	}

	var uid G
	copy(uid[SizeG-len(b):], b)
	return uid, nil
}

// FromTG converts a wall clock instant to a globally unique 96-bit k-ordered
// value.
//
// The instant is placed into the time domain of the clock, so that the value
// sorts against values the clock allocates, see TimeOrder.
func FromTG(clock Chronos, t time.Time) G {
	return makeG(clock.Node(), clock.Drift(), tick(clock.Order(), t), 0)
}
