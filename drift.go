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

import "time"

// Drift is ⟨𝒅⟩, the rung of the ladder that sets the width Δ of the window
// inside which ⟨𝒍⟩ location outranks ⟨𝒕⟩ time.
//
// It is the one policy decision the schema asks for: how much disagreement
// between the clocks of a cluster the k-ordering has to absorb. Two values
// allocated further apart than Δ + 2ε are always ordered by their time; inside
// that window they are ordered by their allocator. A larger Δ tolerates more
// skew, a smaller one yields a tighter k.
//
// The type is the 3-bit code the value carries, not the window itself, so the
// ladder has exactly eight rungs, see the constants below. Read the window of
// a rung with Window, pick a rung for a tolerance with DriftOf.
//
// The drift must be the same for every value of a keyspace: ⟨𝒅⟩ is the most
// significant fraction, so values allocated with different drift are
// segregated rather than interleaved. This is why it is a property of Chronos
// and not an argument of the allocators.
type Drift uint64

// The drift ladder.
//
// ⟨𝒅⟩ is 3 bits, so the ladder has eight rungs, while the layout admits 26
// values of 𝑫. The rungs are therefore chosen rather than contiguous: the
// sub-second range is spent on the ordering classes other schemas occupy, the
// second range on the failover budgets this library was written for.
//
// Each constant is named for its window Δ rounded down, the exact value and
// the deployment it is intended for are given below.
const (
	// Δ ≈ 1.05 ms (𝑫 = 3) — ordering-first. The same ordering class as
	// Snowflake and UUIDv7, with sub-millisecond ties broken by allocator.
	Drift1ms Drift = iota
	// Δ ≈ 16.8 ms (𝑫 = 7) — a single datacenter with disciplined NTP.
	Drift16ms
	// Δ ≈ 268 ms (𝑫 = 11) — multiple regions synchronized over a WAN.
	Drift268ms
	// Δ ≈ 2.15 s (𝑫 = 14) — consumer devices with working time sync.
	Drift2s
	// Δ ≈ 17.2 s (𝑫 = 17) — lease expiry and fast failure detectors.
	Drift17s
	// Δ ≈ 274.9 s (𝑫 = 21) — gossip convergence and unmanaged clocks.
	// The default, see WithDrift.
	Drift275s
	// Δ ≈ 1099 s, 18.3 min (𝑫 = 23) — slow cross-region hand-over.
	Drift1099s
	// Δ ≈ 4398 s, 73.3 min (𝑫 = 25) — human-in-the-loop failover.
	Drift4398s
)

// number of rungs the 3-bit ⟨𝒅⟩ code addresses
const drifts = 1 << bitsDrift

// the rung a clock uses when it was not configured with WithDrift
const driftDefault = Drift275s

// driftLadder maps a ⟨𝒅⟩ code to 𝑫, the number of ⟨𝒕⟩ bits that rank below
// ⟨𝒍⟩. The window is Δ = 2^(17+𝑫) nanoseconds.
//
// The layout admits every 𝑫 ∈ {0,…,25}. The upper bound is the epoch: ⟨𝑬⟩ is
// 47 − 𝑫 bits wide and one narrower than 22 bits no longer spans the range of
// the clock. The lower bound is 𝑫 = 0, where ⟨𝒙ₗ⟩ vanishes and the layout
// degenerates to ⟨𝒅⟩·⟨𝑬⟩·⟨𝒍⟩·⟨𝒔⟩ — Snowflake's field order. Neither bound is a
// property of the machine word: the value is packed positionally, see makeG.
//
// The eight rungs are spent on that range rather than on its bottom. The
// ladder stops at 𝑫 = 3 because the window is Δ + 2ε: below a millisecond the
// clock skew ε decides it and a lower rung buys nothing that a better clock
// does not already have to provide.
var driftLadder = [drifts]uint64{3, 7, 11, 14, 17, 21, 23, 25}

// Bits returns 𝑫, the number of ⟨𝒕⟩ bits that rank below ⟨𝒍⟩ at this rung.
// E.g. 𝑫 = 21 makes the last 21 bits of the truncated timestamp less
// significant than the location.
func (d Drift) Bits() uint64 { return driftLadder[d&(drifts-1)] }

// Window returns Δ, the width of the window inside which ⟨𝒍⟩ outranks ⟨𝒕⟩.
//
// Δ = 2^(17+𝑫) nanoseconds. It is the schema's half of the drift window
// 𝑾 = Δ + 2ε; the other half is the quality of the deployment's clocks.
func (d Drift) Window() time.Duration {
	return time.Duration(1) << (bitsSeqDrift + d.Bits())
}

// String returns the window of the rung, e.g. "274.877906944s".
func (d Drift) String() string { return d.Window().String() }

// DriftOf selects the smallest rung of the ladder whose window Δ covers the
// requested tolerance, so that the k-ordering absorbs at least that much
// clock disagreement. A tolerance above the top rung selects the top rung.
func DriftOf(drift time.Duration) Drift {
	for d := Drift(0); d < drifts-1; d++ {
		if drift <= d.Window() {
			return d
		}
	}

	return drifts - 1
}
