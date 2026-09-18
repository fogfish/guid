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

// place puts a field at bit position p of a value held as the pair (hi, lo),
// so that the field contributes hi·2⁶⁴ + lo to it.
//
// The pair is the base-2⁶⁴ decomposition of the value, whatever the value's
// width: 96 bits for a G, 122 for the payload of an X. Nothing here knows
// which, the caller supplies the positions.
//
// The value is packed positionally, exactly as Proposition 1 of doc/proof.md
// states it, rather than by hand-placed shifts per drift regime. A field is
// therefore laid out by the same expression whether it falls entirely inside
// lo, entirely inside hi, or straddles the boundary between them — which is
// what makes the whole ladder of drifts reachable, see driftLadder.
//
// Shift counts of 64 and above yield zero in Go, so p = 0 and p ≥ 64 need no
// special case.
func place(v, p uint64) (hi, lo uint64) {
	if p >= 64 {
		return v << (p - 64), 0
	}

	return v >> (64 - p), v << p
}

// extract reads the w-bit field at bit position p back out of the pair
// (hi, lo). It is the inverse of place for a field of w bits.
func extract(hi, lo, p, w uint64) uint64 {
	var v uint64
	if p >= 64 {
		v = hi >> (p - 64)
	} else {
		v = lo>>p | hi<<(64-p)
	}

	return v & (1<<w - 1)
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
