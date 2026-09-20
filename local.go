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
// value, to be restored by G.FromL.
func NewL(clock Chronos) L {
	t, seq := clock.T()
	return makeL(clock.Drift(), t, seq)
}

// ZeroL returns the "zero" 64-bit k-ordered value, the value that precedes
// every value allocated by the clock.
func ZeroL(clock Chronos) L {
	return makeL(clock.Drift(), 0, 0)
}

// makeL packs the three fractions into the 64-bit value, positionally:
//
//	⟦makeL⟧ = 𝒅·2⁶¹ + 𝒙·2¹⁴ + 𝒔
//
// which is Proposition 2 of doc/proof.md. Unlike G the local value keeps the
// whole truncated clock ⟨𝒙⟩ above ⟨𝒔⟩ — it is not split around a location
// field, because there is no location field, so the rung of the ladder does
// not affect the layout at all, only what G.FromL restores.
func makeL(drift Drift, t, seq uint64) L {
	d := (uint64(drift) & maskDrift) << (SizeL*8 - bitsDrift)
	x := t >> bitsSeqDrift << bitsSeq

	return L(d | x | seq&maskSeq)
}

// Drift returns the ⟨𝒅⟩ fraction, the rung of the ladder the value was
// allocated with, which ranks ⟨𝒕⟩ against ⟨𝒍⟩ once the value is cast to G. The
// code occupies the 3 most significant bits of the value.
func (uid L) Drift() Drift { return Drift(uint64(uid) >> (SizeL*8 - bitsDrift)) }

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
	return makeL(uid.Drift(), uid.Time()-b.Time(), uid.Seq()-b.Seq())
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

	return uid.FromString(val)
}

// MarshalText implements encoding.TextMarshaler, so that the value travels
// through any codec that speaks it — yaml, toml, a struct tag, a map key.
func (uid L) MarshalText() ([]byte, error) {
	return []byte(uid.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler, see FromString.
func (uid *L) UnmarshalText(b []byte) error {
	return uid.FromString(string(b))
}

// MarshalBinary implements encoding.BinaryMarshaler, see Bytes.
func (uid L) MarshalBinary() ([]byte, error) {
	return uid.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler, see FromBytes.
func (uid *L) UnmarshalBinary(b []byte) error {
	return uid.FromBytes(b)
}

// Fold composes the value from a byte slice. It is the inverse of Split, the
// value n being the number of bits each cell of the slice carries.
func (uid *L) Fold(n uint64, bytes []byte) {
	_, lo := fold(SizeL*8, n, bytes)
	*uid = L(lo)
}

// FromBytes decodes the value from its wire format. It is the inverse of
// Bytes.
func (uid *L) FromBytes(val []byte) error {
	if len(val) != SizeL {
		return fmt.Errorf("malformed k-order number: %v", val)
	}

	*uid = L(binary.BigEndian.Uint64(val))
	return nil
}

// FromString decodes the value from the lexicographically sortable string. It
// is the inverse of String.
func (uid *L) FromString(val string) error {
	if len(val) != SizeString {
		return fmt.Errorf("malformed k-order number: %v", val)
	}

	uid.Fold(4, decode64(val))
	return nil
}

// FromBase62 decodes the value from the base62 string. It is the inverse of
// Base62.
func (uid *L) FromBase62(val string) error {
	b, err := decode62([]byte(val))
	if err != nil {
		return err
	}

	// base62 is a positional numeral system, it does not carry leading zeros
	if len(b) > SizeL {
		return fmt.Errorf("malformed k-order number: %v", val)
	}

	var buf [SizeL]byte
	copy(buf[SizeL-len(b):], b)
	*uid = L(binary.BigEndian.Uint64(buf[:]))
	return nil
}

// FromTime sets the value to a wall clock instant.
//
// The instant is placed into the time domain of the clock, so that the value
// sorts against values the clock allocates, see TimeOrder.
func (uid *L) FromTime(clock Chronos, t time.Time) {
	*uid = makeL(clock.Drift(), tick(clock.Order(), t), 0)
}

// FromG casts a globally unique 96-bit value to this locally unique 64-bit one
// by dropping the ⟨𝒍⟩ fraction.
func (uid *L) FromG(val G) {
	*uid = makeL(val.Drift(), val.Time(), val.Seq())
}
