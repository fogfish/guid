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
	"math/bits"
	"time"
	"unsafe"
)

// X is a globally unique 128-bit k-ordered value that is also a well-formed
// RFC 9562 UUID of version 8.
//
// It carries the same five fractions as G in the same order, so everything
// proven about the ordering of G holds for it, with the ⟨𝒍⟩ location widened
// from 32 to 58 bits:
//
//	3bit  47 bit - 𝒅 bit             58 bit          𝒅 bit  14 bit
//	|-|-------------------|--------------------------|-----|-------|
//	⟨𝒅⟩        ⟨𝒕⟩                    ⟨𝒍⟩               ⟨𝒕⟩     ⟨𝒔⟩
//
// The widening is the point of the type rather than a side effect of its
// width. The birthday bound on randomly allocated node identities moves from
// ≈ 6.5·10⁴ allocators for G to ≈ 5.4·10⁸ for X, which takes node collision
// out of the set of things an operator has to reason about. Ordering is
// proven; uniqueness is the assumption that bites, see §5 of doc/proof.md.
//
// # An identifier other systems understand
//
// Those five fractions occupy 122 bits. The remaining 6 are the version and
// variant fields RFC 9562 fixes, so the value is not merely UUID-shaped: it
// renders as a UUID, parses as a UUID, and drops into a uuid column in
// PostgreSQL, MySQL or SQL Server, into every UUID library of every language,
// and into every debugger and log viewer, rather than showing up as a
// malformed v7. A schema whose premise is allocation across uncoordinated
// nodes should be readable by more than one language, and a cluster of
// uncoordinated nodes is rarely a cluster of uniform Go processes.
//
// The reserved bits cost 6 bits of payload and nothing else. They are
// constants of the format, so at every one of those bit positions two values
// are identical and a most-significant-first comparison falls through to the
// next position: lexicographic order over the 128 bits is exactly
// lexicographic order over the variable payload, in field order. bytes.Compare
// over the raw bytes therefore still agrees with Before, exactly as for G.
//
// # Choosing between X, G and L
//
//	many uncoordinated allocators, interop or node count matters  X  16 B
//	many uncoordinated allocators, storage footprint matters      G  12 B
//	one allocator, or a context that disambiguates                L   8 B
//
// X and G values must never share a keyspace. They are different widths with
// different field semantics and no ordering relation between them is defined;
// convert explicitly at the boundary with G.FromX.
type X [SizeX]byte

// The RFC 9562 constants X carries.
const (
	// version 8, "experimental or vendor-specific use", §5.8
	verX = 0b1000
	// the RFC 9562 variant, §4.1
	varX = 0b10
	// bit position of the version field within the 128-bit value
	posVer = SizeX*8 - 48 - bitsVer
	// bit position of the variant field within the 128-bit value
	posVar = SizeX*8 - 64 - bitsVar
)

// The three runs the version and variant fields cut the payload into.
//
// Counting from the most significant bit, as RFC 9562 does, the version
// occupies bits 48…51 of the value and the variant bits 64…65. Payload bit 𝒑
// therefore lands at value bit 𝒑 for 𝒑 ∈ [0,47], at 𝒑+4 for 𝒑 ∈ [48,59] and at
// 𝒑+6 for 𝒑 ∈ [60,121] — a pure bit permutation, independent of the drift.
//
// Counted the other way round, from the least significant bit, that is three
// contiguous runs of the payload shifted by 0, 2 and 6 bits. The constants
// below name them, since that is the direction the arithmetic runs in.
const (
	// payload [0, 62) stays where it is, below the variant
	runLoX = posVar
	// payload [62, 74) moves up by 2, between the variant and the version
	runMidX = posVer - posVar - bitsVar
	// payload [74, 122) moves up by 6, above the version
	runHiX = bitsPayloadX - runLoX - runMidX
)

// NewX allocates a globally unique 128-bit k-ordered value.
//
// The ⟨𝒅⟩ drift is taken from the clock, see WithDrift, so that every value of
// a keyspace is allocated with the same one. The clock's ⟨𝒍⟩ node identity is
// used at its full 58-bit width, unlike NewG which truncates it to 32 bits.
func NewX(clock Chronos) X {
	t, seq := clock.T()
	return makeX(clock.Node(), clock.Drift(), t, seq)
}

// ZeroX returns the "zero" 128-bit k-ordered value, the value that precedes
// every value allocated by the clock.
//
// Its payload is zero; its bytes are not, since the version and variant fields
// are present in every value of the format.
func ZeroX(clock Chronos) X {
	return makeX(0, clock.Drift(), 0, 0)
}

// makeX packs the five fractions into the 122-bit payload and splices the
// RFC 9562 constants into it.
//
// The packing is a transcription of Guid.pack of doc/proof.lean,
//
//	pack D d E l x s = (((d·2^(47−D) + E)·2⁵⁸ + l)·2^D + x)·2¹⁴ + s
//
// evaluated in Horner form over a 128-bit accumulator, with 2³² replaced by
// 2⁵⁸ — the only edit the wider node asks of the model. Writing it this way
// rather than by placing each fraction at a computed bit position, as makeG
// does, makes the correspondence between the code and the machine-checked
// definition syntactic rather than argued: Proposition 1 for X is an appeal to
// the definition rather than a proof about bit placement.
func makeX(n uint64, drift Drift, t, seq uint64) X {
	d := drift.Bits()
	x := t >> bitsSeqDrift

	v := u128{lo: uint64(drift) & maskDrift}
	v = v.horner(bitsTime-d, x>>d)
	v = v.horner(bitsNodeX, n&maskNodeX)
	v = v.horner(d, x&(1<<d-1))
	v = v.horner(bitsSeq, seq&maskSeq)

	return joinX(splice(v.hi, v.lo))
}

// u128 is the accumulator makeX evaluates the packing polynomial in, a
// 128-bit unsigned integer as the pair (hi, lo) with value hi·2⁶⁴ + lo. Only
// the low bitsPayloadX bits are ever occupied.
type u128 struct{ hi, lo uint64 }

// horner is one step of the Horner form, v·2^k + a for a < 2^k.
//
// No step can overflow 128 bits: the accumulator holds 3, then 50−𝑫, 108−𝑫,
// 108 and finally 122 bits, by the width identity 3+(47−𝑫)+58+𝑫+14 = 122.
func (v u128) horner(k, a uint64) u128 {
	hi, lo := bits.Mul64(v.lo, 1<<k)
	lo, carry := bits.Add64(lo, a, 0)
	hi, _ = bits.Add64(hi, v.hi<<k, carry)

	return u128{hi: hi, lo: lo}
}

// splice interleaves the 122-bit payload with the version and variant fields,
// yielding the 128-bit UUID as the pair (hi, lo).
//
// The three runs of the payload are placed exactly as any other field is, see
// place, and the two constants are placed alongside them. Nothing here depends
// on the drift: the permutation is the same for every rung of the ladder, and
// for every rung both constants fall strictly inside ⟨𝒍⟩, the one fraction
// that is never read arithmetically. ⟨𝒅⟩, ⟨𝑬⟩, ⟨𝒙ₗ⟩ and ⟨𝒔⟩ stay contiguous.
func splice(phi, plo uint64) (hi, lo uint64) {
	lohi, lolo := place(extract(phi, plo, 0, runLoX), 0)
	mihi, milo := place(extract(phi, plo, runLoX, runMidX), posVar+bitsVar)
	hihi, hilo := place(extract(phi, plo, runLoX+runMidX, runHiX), posVer+bitsVer)
	vehi, velo := place(verX, posVer)
	vahi, valo := place(varX, posVar)

	return lohi | mihi | hihi | vehi | vahi, lolo | milo | hilo | velo | valo
}

// payload is the inverse of splice: it drops the version and variant fields
// and closes the gaps, returning the 122-bit payload as the pair (hi, lo).
func (uid X) payload() (uint64, uint64) {
	hi, lo := uid.words()

	lohi, lolo := place(extract(hi, lo, 0, runLoX), 0)
	mihi, milo := place(extract(hi, lo, posVar+bitsVar, runMidX), runLoX)
	hihi, hilo := place(extract(hi, lo, posVer+bitsVer, runHiX), runLoX+runMidX)

	return lohi | mihi | hihi, lolo | milo | hilo
}

// words decomposes the value into the pair (hi, lo) so that the value equals
// hi·2⁶⁴ + lo.
func (uid X) words() (uint64, uint64) {
	return binary.BigEndian.Uint64(uid[0:8]), binary.BigEndian.Uint64(uid[8:16])
}

// joinX is the inverse of words
func joinX(hi, lo uint64) (uid X) {
	binary.BigEndian.PutUint64(uid[0:8], hi)
	binary.BigEndian.PutUint64(uid[8:16], lo)
	return
}

// Drift returns the ⟨𝒅⟩ fraction, the rung of the ladder the value was
// allocated with. The code occupies the 3 most significant bits of the value,
// as it does in G and L — the first reserved field of the UUID sits well below
// it, at bit 48.
func (uid X) Drift() Drift { return Drift(uid[0] >> (8 - bitsDrift)) }

// Equal compares k-ordered values, returns true if values are equal
func (uid X) Equal(b X) bool { return uid == b }

// Before checks if k-ordered value A is before value B
func (uid X) Before(b X) bool {
	ahi, alo := uid.words()
	bhi, blo := b.words()
	return ahi < bhi || (ahi == bhi && alo < blo)
}

// After checks if k-ordered value A is after value B
func (uid X) After(b X) bool {
	ahi, alo := uid.words()
	bhi, blo := b.words()
	return ahi > bhi || (ahi == bhi && alo > blo)
}

// Time returns ⟨𝒕⟩ timestamp fraction from identifier in nano seconds.
//
// ⟨𝒕⟩ is split around ⟨𝒍⟩: the epoch ⟨𝑬⟩ = ⌊𝒙/2^𝑫⌋ ranks above the location
// and the low bits ⟨𝒙ₗ⟩ = 𝒙 mod 2^𝑫 below it, see makeX.
//
//	  3      47 − 𝑫            58            𝑫       14
//	|---|--------------|------------------|-------|--------|
//	 ⟨𝒅⟩      ⟨𝑬⟩              ⟨𝒍⟩          ⟨𝒙ₗ⟩      ⟨𝒔⟩
func (uid X) Time() uint64 {
	hi, lo := uid.payload()
	d := uid.Drift().Bits()

	e := extract(hi, lo, bitsSeq+d+bitsNodeX, bitsTime-d)
	x := extract(hi, lo, bitsSeq, d)

	return (e<<d | x) << bitsSeqDrift
}

// Node returns ⟨𝒍⟩ location fraction from identifier, at the full 58 bits X
// gives it.
func (uid X) Node() uint64 {
	hi, lo := uid.payload()
	d := uid.Drift().Bits()

	return extract(hi, lo, bitsSeq+d, bitsNodeX)
}

// Seq returns ⟨𝒔⟩ sequence value. The value of monotonic unique integer
// at the time of k-ordered value creation.
func (uid X) Seq() uint64 {
	return uint64(binary.BigEndian.Uint16(uid[14:16]) & maskSeq)
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
func (uid X) Epoch() time.Time {
	return epoch(uid.Time())
}

// Diff approximates distance between k-ordered values.
func (uid X) Diff(b X) X {
	return makeX(uid.Node(), uid.Drift(), uid.Time()-b.Time(), uid.Seq()-b.Seq())
}

// FromG casts a globally unique 96-bit value to this 128-bit one.
//
// The conversion is exact but it does not invent node identity: ⟨𝒍⟩ keeps the
// 32 bits it had, the 26 bits X adds are zero, and so is the birthday bound of
// the original value. Only values allocated by NewX carry a 58-bit node.
func (uid *X) FromG(val G) error {
	*uid = makeX(val.Node(), val.Drift(), val.Time(), val.Seq())
	return nil
}

// FromL casts a locally unique 64-bit value to this globally unique 128-bit
// one by stamping it with the ⟨𝒍⟩ fraction of the clock.
func (uid *X) FromL(clock Chronos, val L) error {
	*uid = makeX(clock.Node(), val.Drift(), val.Time(), val.Seq())
	return nil
}

// FromX casts a globally unique 128-bit value to this compact 96-bit one.
//
// The conversion is lossy: ⟨𝒍⟩ has to narrow from 58 bits to the 32 that G
// gives it, and two nodes that differ only above bit 32 would collapse onto
// the same G. Rather than let that collision happen silently, the cast fails
// when val's node does not fit in 32 bits — the application has to resolve
// the conflict, e.g. by reassigning the colliding node, before the two can
// share a G keyspace. The two types must not share a keyspace in any case,
// see X.
func (uid *G) FromX(val X) error {
	if n := val.Node(); n > maskNode {
		return fmt.Errorf("node identity %#x of X does not fit the 32 bits of G", n)
	}

	*uid = makeG(val.Node(), val.Drift(), val.Time(), val.Seq())
	return nil
}

// FromX casts a globally unique 128-bit value to this locally unique 64-bit
// one by dropping the ⟨𝒍⟩ fraction.
func (uid *L) FromX(val X) error {
	*uid = makeL(val.Drift(), val.Time(), val.Seq())
	return nil
}

// Bytes encodes k-ordered value to byte slice.
//
// The value is already its own encoding, uid[:] is the same bytes without the
// copy. Use it on the hot path when the slice does not outlive the value.
func (uid X) Bytes() []byte {
	b := make([]byte, SizeX)
	copy(b, uid[:])
	return b
}

// Split decomposes the value to bytes slice. The function acts as binary
// comprehension, the value n defines number of bits to extract into each cell.
// It has to divide 128.
func (uid X) Split(n uint64) []byte {
	hi, lo := uid.words()
	b := make([]byte, SizeX*8/n)
	split(hi, lo, SizeX*8, n, b)
	return b
}

// String encodes the value into the canonical UUID string,
// xxxxxxxx-xxxx-8xxx-yxxx-xxxxxxxxxxxx.
//
// Unlike G and L, which use a private alphabet chosen so that the string sorts
// the way the value does, X emits the form every other system recognises —
// that recognition being the whole reason for the type. The canonical form is
// hexadecimal and big-endian, so it sorts correctly anyway, as long as the
// comparison is case sensitive and the dashes line up, which for a fixed-width
// encoding they do. Base62 remains available as the compact representation.
func (uid X) String() string {
	var enc [SizeStringX]byte

	i := 0
	for p, b := range uid {
		switch p {
		case 4, 6, 8, 10:
			enc[i] = '-'
			i++
		}
		enc[i] = hexdigit[b>>4]
		enc[i+1] = hexdigit[b&0xf]
		i += 2
	}

	str := enc[:]
	return *(*string)(unsafe.Pointer(&str))
}

const hexdigit = "0123456789abcdef"

// Base62 encodes k-ordered value to a lexicographically sortable base62
// string. The output is zero-padded to a fixed width per type, which is what
// makes it sortable: a positional numeral system only orders lexicographically
// at a fixed width, since a shorter, unpadded string can otherwise sort after
// a longer one representing a larger value.
func (uid X) Base62() string {
	str := encode62(uid[:])
	return *(*string)(unsafe.Pointer(&str))
}

// MarshalJSON encodes k-ordered value to the canonical UUID string
func (uid X) MarshalJSON() ([]byte, error) {
	return json.Marshal(uid.String())
}

// UnmarshalJSON decodes the canonical UUID string to k-ordered value
func (uid *X) UnmarshalJSON(b []byte) error {
	var val string
	if err := json.Unmarshal(b, &val); err != nil {
		return err
	}

	return uid.FromString(val)
}

// MarshalText implements encoding.TextMarshaler. It is what makes the value
// render as a UUID in yaml, toml, a struct tag or a map key, and not only in
// JSON.
func (uid X) MarshalText() ([]byte, error) {
	return []byte(uid.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler, see FromString.
func (uid *X) UnmarshalText(b []byte) error {
	return uid.FromString(string(b))
}

// MarshalBinary implements encoding.BinaryMarshaler, see Bytes.
func (uid X) MarshalBinary() ([]byte, error) {
	return uid.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler, see FromBytes.
func (uid *X) UnmarshalBinary(b []byte) error {
	return uid.FromBytes(b)
}

// Fold composes the value from a byte slice. It is the inverse of Split, the
// value n being the number of bits each cell of the slice carries.
func (uid *X) Fold(n uint64, bytes []byte) {
	hi, lo := fold(SizeX*8, n, bytes)
	*uid = joinX(hi, lo)
}

// validX reports whether buf carries the RFC 9562 version and variant fields
// every value this package builds is spliced with, see splice. A decoder
// checks it so that a corrupted or foreign payload that happens to be the
// right length is not accepted as if NewX had produced it — the one guardrail
// against garbage input this format admits; G and L carry no such marker, see
// their own FromBytes and FromBase62.
func validX(buf X) bool {
	return buf[6]>>(8-bitsVer) == verX && buf[8]>>(8-bitsVar) == varX
}

// FromBytes decodes the value from its wire format. It is the inverse of
// Bytes.
//
// The version and variant fields are checked, see FromString.
func (uid *X) FromBytes(val []byte) error {
	if len(val) != SizeX {
		return fmt.Errorf("malformed k-order number: %v", val)
	}

	var buf X
	copy(buf[:], val)
	if !validX(buf) {
		return fmt.Errorf("not a RFC 9562 UUIDv8: %v", val)
	}

	*uid = buf
	return nil
}

// FromString decodes the value from the canonical UUID string, in either case,
// e.g. "06377f2a-0cb8-8000-8000-000000000001". It is the inverse of String.
//
// The version and variant fields are checked: a UUID of another version does
// not carry these fractions at these positions, so decoding one would report
// a time and a node that were never allocated. It is not checked, because it
// cannot be, that a v8 UUID was minted by this library rather than by another
// application of the same version.
func (uid *X) FromString(val string) error {
	if len(val) != SizeStringX ||
		val[8] != '-' || val[13] != '-' || val[18] != '-' || val[23] != '-' {
		return fmt.Errorf("malformed k-order number: %v", val)
	}

	var buf X
	for i, p := range [SizeX]int{0, 2, 4, 6, 9, 11, 14, 16, 19, 21, 24, 26, 28, 30, 32, 34} {
		hi, lo := unhex(val[p]), unhex(val[p+1])
		if hi > 0xf || lo > 0xf {
			return fmt.Errorf("malformed k-order number: %v", val)
		}
		buf[i] = hi<<4 | lo
	}

	if !validX(buf) {
		return fmt.Errorf("not a RFC 9562 UUIDv8: %v", val)
	}

	*uid = buf
	return nil
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0xff
}

// FromBase62 decodes the value from the base62 string. It is the inverse of
// Base62.
//
// The version and variant fields are checked, see FromString.
func (uid *X) FromBase62(val string) error {
	b, err := decode62([]byte(val))
	if err != nil {
		return err
	}

	// base62 is a positional numeral system, it does not carry leading zeros
	if len(b) > SizeX {
		return fmt.Errorf("malformed k-order number: %v", val)
	}

	var buf X
	copy(buf[SizeX-len(b):], b)
	if !validX(buf) {
		return fmt.Errorf("not a RFC 9562 UUIDv8: %v", val)
	}

	*uid = buf
	return nil
}

// FromTime sets the value to a wall clock instant.
//
// The instant is placed into the time domain of the clock, so that the value
// sorts against values the clock allocates, see TimeOrder.
func (uid *X) FromTime(clock Chronos, t time.Time) {
	*uid = makeX(clock.Node(), clock.Drift(), tick(clock.Order(), t), 0)
}
