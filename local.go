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

// L is a locally unique 64-bit k-ordered value. It carries no ⟨𝒍⟩ location
// fraction, values are therefore unique only within the allocator that
// produced them.
//
//	3bit        47 bit           14 bit
//	|-|------------------------|-------|
//	⟨𝒅⟩           ⟨𝒕⟩              ⟨𝒔⟩
//
// The type is the 64-bit number itself, one machine word. It is passed in
// registers, compared with a single instruction and stored in 8 bytes, so it
// costs no more than the uint64 an application would otherwise use as a
// surrogate key.
type L uint64

// NewL allocates locally unique 64-bit k-ordered value.
//
// The ⟨𝒅⟩ drift is taken from the clock, see WithDrift. A local value has no
// ⟨𝒍⟩ fraction for the drift to rank against, so the code only travels with the
// value, to be restored by ToG.
func NewL(clock Chronos) L {
	t, seq := clock.T()
	return makeL(clock.Drift(), t, seq)
}

// ZeroL returns the "zero" 64-bit k-ordered value, the value that precedes
// every value allocated by the clock.
func ZeroL(clock Chronos) L {
	return makeL(clock.Drift(), 0, 0)
}

func makeL(drift, t, seq uint64) L {
	d := (drift - driftZ) << (64 - bitsDrift)
	x := t >> bitsSeqDrift << bitsSeq

	return L(d | x | seq)
}

// drift returns the ⟨𝒅⟩ fraction as the number of bits of ⟨𝒕⟩ that rank below
// ⟨𝒍⟩ once the value is cast to G. The code occupies the 3 most significant
// bits of the value.
func (uid L) drift() uint64 { return uint64(uid)>>(64-bitsDrift) + driftZ }

// Equal compares k-ordered values, returns true if values are equal
func (uid L) Equal(b L) bool { return uid == b }

// Before checks if k-ordered value A is before value B
func (uid L) Before(b L) bool { return uid < b }

// After checks if k-ordered value A is after value B
func (uid L) After(b L) bool { return uid > b }

// Time returns ⟨𝒕⟩ timestamp fraction from identifier in nano seconds
func (uid L) Time() uint64 {
	return uint64(uid) << bitsDrift >> bitsSeqDrift << bitsSeqDrift
}

// Seq returns ⟨𝒔⟩ sequence value. The value of monotonic unique integer
// at the time of k-ordered value creation.
func (uid L) Seq() uint64 { return uint64(uid) & maskSeq }

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
func (uid L) Epoch() time.Time {
	return epoch(uid.Time())
}

// Diff approximates distance between k-ordered values.
func (uid L) Diff(b L) L {
	return makeL(uid.drift(), uid.Time()-b.Time(), uid.Seq()-b.Seq())
}

// ToG casts locally unique 64-bit value to globally unique 96-bit one by
// stamping it with the ⟨𝒍⟩ fraction of the clock.
func (uid L) ToG(clock Chronos) G {
	return makeG(clock.Node(), uid.drift(), uid.Time(), uid.Seq())
}

// Bytes encodes k-ordered value to byte slice
func (uid L) Bytes() []byte {
	b := make([]byte, SizeL)
	binary.BigEndian.PutUint64(b, uint64(uid))
	return b
}

// Split decomposes the value to bytes slice. The function acts as binary
// comprehension, the value n defines number of bits to extract into each cell.
func (uid L) Split(n uint64) []byte {
	b := make([]byte, SizeL*8/n)
	split(0, uint64(uid), SizeL*8, n, b)
	return b
}

// String encodes k-ordered value to lexicographically sortable string
func (uid L) String() string {
	var (
		buf [SizeString]byte // interim buffer where uid is split as seq of bytes
		enc [SizeString]byte // output encoded string
	)

	split(0, uint64(uid), SizeL*8, 4, buf[:])

	encode64(buf, &enc)
	str := enc[:]
	return *(*string)(unsafe.Pointer(&str))
}

// Base62 encodes k-ordered value to lexicographically sortable base62 string
func (uid L) Base62() string {
	str := encode62(uid.Bytes())
	return *(*string)(unsafe.Pointer(&str))
}

// MarshalJSON encodes k-ordered value to lexicographically sortable JSON string
func (uid L) MarshalJSON() ([]byte, error) {
	return json.Marshal(uid.String())
}

// UnmarshalJSON decodes lexicographically sortable string to k-ordered value
func (uid *L) UnmarshalJSON(b []byte) error {
	var val string
	if err := json.Unmarshal(b, &val); err != nil {
		return err
	}

	v, err := FromStringL(val)
	if err != nil {
		return err
	}

	*uid = v
	return nil
}

// FoldL composes k-ordered value from byte slice. The operation is inverse
// to Split.
func FoldL(n uint64, bytes []byte) L {
	_, lo := fold(SizeL*8, n, bytes)
	return L(lo)
}

// FromBytesL decodes k-ordered value from bytes
func FromBytesL(val []byte) (L, error) {
	if len(val) != SizeL {
		return 0, fmt.Errorf("malformed k-order number: %v", val)
	}

	return L(binary.BigEndian.Uint64(val)), nil
}

// FromStringL decodes k-ordered value from lexicographically sortable string
func FromStringL(val string) (L, error) {
	if len(val) != SizeString {
		return 0, fmt.Errorf("malformed k-order number: %v", val)
	}

	return FoldL(4, decode64(val)), nil
}

// FromBase62L decodes k-ordered value from base62 string
func FromBase62L(val string) (L, error) {
	b, err := decode62([]byte(val))
	if err != nil {
		return 0, err
	}

	// base62 is a positional numeral system, it does not carry leading zeros
	if len(b) > SizeL {
		return 0, fmt.Errorf("malformed k-order number: %v", val)
	}

	var buf [SizeL]byte
	copy(buf[SizeL-len(b):], b)
	return L(binary.BigEndian.Uint64(buf[:])), nil
}

// FromTL converts a wall clock instant to a locally unique 64-bit k-ordered value.
//
// The instant is placed into the time domain of the clock, so that the value
// sorts against values the clock allocates, see TimeOrder.
func FromTL(clock Chronos, t time.Time) L {
	return makeL(clock.Drift(), tick(clock.Order(), t), 0)
}
