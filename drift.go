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
// It is the one policy decision the schema asks for, and it is a failover
// budget: Δ has to cover the interval between a silent failure and the moment
// the cluster has converged on a new owner, because that is the interval
// during which two allocators write to the same range and their output has to
// stay apart. Two values allocated further apart than Δ + 2ε are always
// ordered by their time; inside that window they are ordered by their
// allocator, each allocator's values forming one contiguous run.
//
// The same number is the clock disagreement the ordering tolerates, which is
// why one knob serves both. A larger Δ attributes a longer overlap, a smaller
// one yields a tighter k.
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
// ⟨𝒅⟩ is 3 bits, so the ladder has eight rungs while the layout admits every
// 𝑫 ∈ {0,…,46}. The rungs are therefore chosen rather than contiguous, and
// they are chosen as failover budgets: seven of the eight lie between a
// consensus election and a split brain found the next morning, the eighth is
// the floor, for a deployment that wants ordering and no attribution at all.
//
// Two rungs are only operationally distinct where the wider Δ is large against
// 2ε, since the window is Δ + 2ε — which is why the ladder does not subdivide
// the sub-second range: below a second the rung stops deciding the window and
// the quality of the deployment's clocks decides it instead.
//
// Each constant is named for its window Δ rounded down, the exact value and
// the deployment it is intended for are given below.
const (
	// Δ ≈ 131 µs (𝑫 = 0) — ordering only, no attribution. ⟨𝒙ₗ⟩ vanishes and
	// the field order degenerates to ⟨𝒅⟩·⟨𝑬⟩·⟨𝒍⟩·⟨𝒔⟩, which is Snowflake's;
	// the window is 2ε, the tightest any coordination-free schema reaches.
	// A run is one tick wide, so this rung buys no attribution — pick it to
	// track real time, not to tell two writers apart.
	Drift131us Drift = iota
	// Δ ≈ 2.15 s (𝑫 = 14) — a consensus election plus lease expiry, Raft or
	// etcd in one datacenter. The lowest rung with a failover story.
	Drift2s
	// Δ ≈ 17.2 s (𝑫 = 17) — gossip convergence, ZooKeeper and Consul
	// sessions, fast failure detectors.
	Drift17s
	// Δ ≈ 68.7 s (𝑫 = 19) — Kubernetes node-NotReady plus reschedule, Kafka
	// session timeout, load balancer health-check chains.
	Drift68s
	// Δ ≈ 274.9 s (𝑫 = 21) — automated cross-AZ hand-over, phi-accrual
	// detection, unmanaged clocks. The default, see WithDrift.
	Drift275s
	// Δ ≈ 1099 s, 18.3 min (𝑫 = 23) — slow membership convergence, paging,
	// cross-region hand-over.
	Drift1099s
	// Δ ≈ 4398 s, 73.3 min (𝑫 = 25) — on-call human-in-the-loop failover.
	Drift4398s
	// Δ ≈ 140737 s, 39.1 h (𝑫 = 30) — a split brain discovered the next
	// morning, a fleet that syncs once a day, a region isolated for a
	// working day. A time range narrower than Δ costs one seek per ⟨𝒍⟩
	// rather than one contiguous range, and only an assigned ⟨𝒍⟩ can be
	// enumerated to make those seeks, see WithNodeID.
	Drift39h
)

// number of rungs the 3-bit ⟨𝒅⟩ code addresses
const drifts = 1 << bitsDrift

// the rung a clock uses when it was not configured with WithDrift
const driftDefault = Drift275s

// driftLadder maps a ⟨𝒅⟩ code to 𝑫, the number of ⟨𝒕⟩ bits that rank below
// ⟨𝒍⟩. The window is Δ = 2^(17+𝑫) nanoseconds.
//
// The layout admits every 𝑫 ∈ {0,…,46} and neither bound is a property of the
// machine word: the value is packed positionally, see makeG. At 𝑫 = 0 there
// are no low clock bits left to place and the layout degenerates to
// Snowflake's field order; at 𝑫 = 47 the epoch vanishes and ⟨𝒍⟩ outranks time
// altogether, so 𝑫 = 46 is the last rung that still orders by time at all.
//
// The span of the clock does not bound the ladder. It is invariant in 𝑫:
// 2^(47−𝑫) epochs of 2^(17+𝑫) ns is 2⁶⁴ ns, about 584 years, whatever 𝑫 is —
// a narrower ⟨𝑬⟩ counts proportionally wider epochs. Versions of this library
// up to v3 stopped the ladder at 𝑫 = 25 and gave the span as the reason; the
// reason was arithmetically empty and the value was inherited from v2, whose
// codes meant 𝑫 = 18 + code.
//
// What does bound it is the meaning of Δ. Above a day or so the window stops
// being a failover budget and becomes all of time: every value of a
// deployment lands in one epoch, so the partition by ⟨𝒍⟩ discriminates
// nothing, while k = ρ·(Δ + 2ε) grows without any return. The top rung is
// therefore 𝑫 = 30, which covers a 24 h overlap with margin.
var driftLadder = [drifts]uint64{0, 14, 17, 19, 21, 23, 25, 30}

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
// requested budget, so that the k-ordering absorbs at least that much.
//
// The budget is a failover interval — detection plus convergence, the longest
// single overlap two owners of one range can have — and never less than the
// deployment's worst clock skew ε, since a rung below ε is decided by the
// clocks rather than by the setting. A budget above the top rung selects the
// top rung.
func DriftOf(drift time.Duration) Drift {
	for d := Drift(0); d < drifts-1; d++ {
		if drift <= d.Window() {
			return d
		}
	}

	return drifts - 1
}
