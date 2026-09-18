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
	"time"
)

// zero point for drift.
//
// ⟨𝒅⟩ = 𝑫 − driftZ, and the 96-bit layout places the 32-bit ⟨𝒍⟩ fraction so
// that its top 𝑫 − 18 bits fall above the Hi/Lo boundary, see splitNode. 𝑫 = 18
// is therefore the smallest drift the layout admits: ⟨𝒍⟩ then occupies exactly
// the top 32 bits of Lo and ⟨𝑬⟩ the 29 bits below ⟨𝒅⟩. A smaller 𝑫 would spill
// ⟨𝑬⟩ across the word boundary, which splitT and splitNode do not express.
// The floor is geometry, not policy.
const driftZ = 18

// the drift assumed by a clock that was not configured with WithDrift
const driftDefault = 274 * time.Second

// driftInBits converts a time drift into the number of ⟨𝒕⟩ bits that rank
// below ⟨𝒍⟩. E.g. if the application tolerates 2 min of clock disagreement
// then the last 20 bits of the timestamp become less significant than the
// location.
//
// The code is stored as 3 bits, so the ladder has 8 rungs. It is bounded below
// by driftZ, the smallest drift the 96-bit layout admits, and the rung spacing
// is a factor of two because ⟨𝒅⟩ selects a bit position.
//
// The drift a value was allocated with must be constant across a keyspace:
// ⟨𝒅⟩ is the most significant fraction, so values allocated with different
// drift are segregated rather than interleaved. This is why the drift is a
// property of Chronos and not an argument of the allocators.
func driftInBits(drift time.Duration) uint64 {
	switch {
	case drift <= 34*time.Second:
		return driftZ
	case drift <= 68*time.Second:
		return driftZ + 1
	case drift <= 137*time.Second:
		return driftZ + 2
	case drift <= 274*time.Second:
		return driftZ + 3
	case drift <= 549*time.Second:
		return driftZ + 4
	case drift <= 1099*time.Second:
		return driftZ + 5
	case drift <= 2199*time.Second:
		return driftZ + 6
	default:
		return driftZ + 7
	}
}

// epoch maps a ⟨𝒕⟩ fraction back to the wall clock instant it was allocated at.
//
// The domain the value belongs to is recovered from ⟨𝒕⟩ itself, no clock is
// needed. A descending tick is MaxUint64 − UnixNano, which is ≥ 2⁶³ exactly
// while UnixNano ≤ 2⁶³; an ascending tick is < 2⁶³ under the same condition.
// The test therefore separates the two domains exactly, and it is decided by
// the same bit that decides the sign of the int64 nanosecond count below. The
// discrimination inverts on 2262-04-11, the day int64 nanoseconds overflow, so
// it expires with the return type rather than before it.
//
// ⟨𝒕⟩ carries no ⟨𝒔⟩ of its own and is truncated by bitsSeqDrift bits, so the
// instant is accurate to one tick, 2¹⁷ ns ≈ 131 µs. Truncation floors the tick,
// which rounds an ascending instant down and a descending one up.
func epoch(t uint64) time.Time {
	if t >= 1<<63 {
		return time.Unix(0, int64(^uint64(0)-t))
	}

	return time.Unix(0, int64(t))
}

// tick maps a wall clock instant into the ⟨𝒕⟩ domain of a clock.
//
// It is the inverse of epoch. Unlike epoch it needs the direction: a time.Time
// carries no domain, so the clock that owns the keyspace has to supply it.
func tick(order TimeOrder, t time.Time) uint64 {
	if order == InverseTime {
		return ^uint64(0) - uint64(t.UnixNano())
	}

	return uint64(t.UnixNano())
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

func split(hi, lo, size, n uint64, bytes []byte) {
	hilo := uint64(64) // hi | lo division at
	mask := uint64(1<<n) - 1
	i := 0

	for a := size; a >= n; a -= n {
		b := a - n
		switch {
		case a >= hilo && b >= hilo:
			bytes[i] = byte(hi >> (b - hilo) & mask)
		case a <= hilo && b <= hilo:
			bytes[i] = byte(lo >> b & mask)
		case a > hilo && b < hilo:
			suffix := uint64(1<<(a-hilo)) - 1
			hi := byte(hi & suffix)
			lo := byte(lo >> b)
			bytes[i] = hi<<(hilo-b) | lo
		}
		i++
	}
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
