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
//
// Unlike X, G reserves no version or variant field: every 96-bit number is a
// syntactically valid G, so FromBytes, FromString and FromBase62 can reject a
// malformed length but not a garbage payload of the right one — there is no
// bit pattern left for the schema to check, and widening the layout to make
// room for one would cost every value of the type, not only the decoded ones.
// An application that must tell a genuine G from arbitrary data has to keep
// that guarantee on its own side, e.g. by wrapping G in a type nothing outside
// this package can construct, or by storing a provenance tag alongside it.
// Reach for X when that validation matters more than the footprint; its
// FromString, FromBytes and FromBase62 all check the version and variant
// RFC 9562 fixes.
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

// makeG packs the five fractions into the 96-bit value, positionally:
//
//	⟦makeG⟧ = 𝒅·2⁹³ + 𝑬·2^(46+𝑫) + 𝒍·2^(14+𝑫) + 𝒙ₗ·2¹⁴ + 𝒔
//
// which is Proposition 1 of doc/proof.md written out. Each fraction is placed
// at the bit position the schema gives it and the results are OR-ed; the
// ranges are disjoint by the width identity 3 + (47−𝑫) + 32 + 𝑫 + 14 = 96, so
// the OR is an addition and the positional form is the definition rather than
// a consequence of shift arithmetic. Every rung of the ladder is therefore
// reachable, including those where ⟨𝑬⟩ spills across the hi/lo boundary of the
// two machine words the value is assembled from.
func makeG(n uint64, drift Drift, t, seq uint64) G {
	d := drift.Bits()
	x := t >> bitsSeqDrift

	dhi, dlo := place(uint64(drift)&maskDrift, posDrift)
	ehi, elo := place(x>>d, bitsSeq+d+bitsNode)
	nhi, nlo := place(n&maskNode, bitsSeq+d)
	xhi, xlo := place(x&(1<<d-1), bitsSeq)

	return joinG(dhi|ehi|nhi|xhi, dlo|elo|nlo|xlo|seq&maskSeq)
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

// Drift returns the ⟨𝒅⟩ fraction, the rung of the ladder the value was
// allocated with. The code occupies the 3 most significant bits of the value.
func (uid G) Drift() Drift { return Drift(uid[0] >> (8 - bitsDrift)) }

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

// Time returns ⟨𝒕⟩ timestamp fraction from identifier in nano seconds.
//
// ⟨𝒕⟩ is split around ⟨𝒍⟩: the epoch ⟨𝑬⟩ = ⌊𝒙/2^𝑫⌋ ranks above the location
// and the low bits ⟨𝒙ₗ⟩ = 𝒙 mod 2^𝑫 below it, see makeG.
//
//	  3      47 − 𝑫        32          𝑫       14
//	|---|--------------|------------|-------|--------|
//	 ⟨𝒅⟩      ⟨𝑬⟩           ⟨𝒍⟩       ⟨𝒙ₗ⟩      ⟨𝒔⟩
func (uid G) Time() uint64 {
	hi, lo := uid.words()
	d := uid.Drift().Bits()

	e := extract(hi, lo, bitsSeq+d+bitsNode, bitsTime-d)
	x := extract(hi, lo, bitsSeq, d)

	return (e<<d | x) << bitsSeqDrift
}

// Node returns ⟨𝒍⟩ location fraction from identifier.
func (uid G) Node() uint64 {
	hi, lo := uid.words()
	d := uid.Drift().Bits()

	return extract(hi, lo, bitsSeq+d, bitsNode)
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
	return makeG(uid.Node(), uid.Drift(), uid.Time()-b.Time(), uid.Seq()-b.Seq())
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

// Base62 encodes k-ordered value to a lexicographically sortable base62
// string. The output is zero-padded to a fixed width per type, which is what
// makes it sortable: a positional numeral system only orders lexicographically
// at a fixed width, since a shorter, unpadded string can otherwise sort after
// a longer one representing a larger value.
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

	return uid.FromString(val)
}

// MarshalText implements encoding.TextMarshaler, so that the value travels
// through any codec that speaks it — yaml, toml, a struct tag, a map key.
func (uid G) MarshalText() ([]byte, error) {
	return []byte(uid.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler, see FromString.
func (uid *G) UnmarshalText(b []byte) error {
	return uid.FromString(string(b))
}

// MarshalBinary implements encoding.BinaryMarshaler, see Bytes.
func (uid G) MarshalBinary() ([]byte, error) {
	return uid.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler, see FromBytes.
func (uid *G) UnmarshalBinary(b []byte) error {
	return uid.FromBytes(b)
}

// Fold composes the value from a byte slice. It is the inverse of Split, the
// value n being the number of bits each cell of the slice carries.
func (uid *G) Fold(n uint64, bytes []byte) {
	hi, lo := fold(SizeG*8, n, bytes)
	*uid = joinG(hi, lo)
}

// FromBytes decodes the value from its wire format. It is the inverse of
// Bytes.
func (uid *G) FromBytes(val []byte) error {
	if len(val) != SizeG {
		return fmt.Errorf("malformed k-order number: %v", val)
	}

	copy(uid[:], val)
	return nil
}

// FromString decodes the value from the lexicographically sortable string. It
// is the inverse of String.
func (uid *G) FromString(val string) error {
	if len(val) != SizeString {
		return fmt.Errorf("malformed k-order number: %v", val)
	}

	uid.Fold(6, decode64(val))
	return nil
}

// FromBase62 decodes the value from the base62 string. It is the inverse of
// Base62.
func (uid *G) FromBase62(val string) error {
	b, err := decode62([]byte(val))
	if err != nil {
		return err
	}

	// base62 is a positional numeral system, it does not carry leading zeros
	if len(b) > SizeG {
		return fmt.Errorf("malformed k-order number: %v", val)
	}

	*uid = G{}
	copy(uid[SizeG-len(b):], b)
	return nil
}

// FromTime sets the value to a wall clock instant.
//
// The instant is placed into the time domain of the clock, so that the value
// sorts against values the clock allocates, see TimeOrder.
func (uid *G) FromTime(clock Chronos, t time.Time) {
	*uid = makeG(clock.Node(), clock.Drift(), tick(clock.Order(), t), 0)
}

// FromL casts a locally unique 64-bit value to this globally unique 96-bit one
// by stamping it with the ⟨𝒍⟩ fraction of the clock.
func (uid *G) FromL(clock Chronos, val L) {
	*uid = makeG(clock.Node(), val.Drift(), val.Time(), val.Seq())
}
