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

import "sync/atomic"

// cacheLine keeps the read-mostly ⟨𝒕⟩ guard and the hot ⟨𝒔⟩ counter of a
// sequence on distinct cache lines. 128 bytes covers every architecture the
// library targets (64 byte lines on amd64, 128 byte lines on arm64).
const cacheLine = 128

// sequence allocates the ⟨𝒕,𝒔⟩ fraction of k-ordered values.
//
// The ordering of k-ordered values within a single allocator is decided by the
// pair ⟨𝒕,𝒔⟩ compared lexicographically, which is the same thing as comparing
// the single number
//
//	𝑽 = ⟨𝒕⟩ · 2¹⁴ + ⟨𝒔⟩
//
// Values are ordered as they are allocated if and only if 𝑽 strictly increases.
// Deriving ⟨𝒕⟩ and ⟨𝒔⟩ from two independent sources cannot guarantee that: a
// free running ⟨𝒔⟩ counter folds back to 0 every 2¹⁴ allocations, and if that
// happens while ⟨𝒕⟩ stands still — ⟨𝒕⟩ only advances once per 2¹⁷ nanoseconds —
// the two values are allocated in one order and sort in the other. Note that
// staying below 2¹⁴ allocations per ⟨𝒕⟩ does not help: what breaks the order is
// crossing a multiple of 2¹⁴, not the number of allocations between two of them.
//
// sequence therefore keeps 𝑽 itself, as one word, and moves it in one direction
// only:
//
//	allocate: 𝑽 ← 𝑽 + 1                       (always, one atomic add)
//	advance:  𝑽 ← max(𝑽, ⟨𝒕⟩ · 2¹⁴)           (when the clock ticks)
//
// ⟨𝒔⟩ is then the low 14 bits of 𝑽 and restarts on every tick, while an
// allocator that exhausts a tick simply carries into the next one instead of
// folding back. Monotonicity is a property of the construction rather than an
// assumption about allocation rates, so it holds for any clock — including one
// that is stepped backwards, where 𝑽 stays at its high water mark until real
// time catches up.
//
// The cost is the same single atomic add on the allocation path. The ⟨𝒕⟩ guard
// is read on every allocation but written only when the clock ticks, about
// 7600 times per second, so it stays valid in the local cache of every core.
type sequence struct {
	// last observed ⟨𝒕⟩, scaled to the units of 𝑽; read-mostly
	tick atomic.Uint64
	_    [cacheLine - 8]byte
	// 𝑽, the packed ⟨𝒕,𝒔⟩ pair; the hot word
	v atomic.Uint64
	_ [cacheLine - 8]byte
}

// next allocates 𝑽 from an ascending clock, e.g. a unix timestamp.
// The value returned is strictly greater than every value returned before it.
func (seq *sequence) next(t uint64) uint64 {
	base := t >> bitsSeqDrift << bitsSeq

	if tick := seq.tick.Load(); base > tick {
		// the clock has ticked; exactly one allocator pulls 𝑽 up to it
		if seq.tick.CompareAndSwap(tick, base) {
			for {
				v := seq.v.Load()
				if v >= base || seq.v.CompareAndSwap(v, base) {
					break
				}
			}
		}
	}

	return seq.v.Add(1)
}

// prev allocates 𝑽 from a descending clock, e.g. an inverse unix timestamp.
// The value returned is strictly less than every value returned before it.
// It is the mirror of next: ⟨𝒔⟩ counts down from 2¹⁴-1 within a tick and
// borrows from the next (lower) tick when exhausted.
func (seq *sequence) prev(t uint64) uint64 {
	base := t>>bitsSeqDrift<<bitsSeq + 1<<bitsSeq

	if tick := seq.tick.Load(); base < tick {
		if seq.tick.CompareAndSwap(tick, base) {
			for {
				v := seq.v.Load()
				if v <= base || seq.v.CompareAndSwap(v, base) {
					break
				}
			}
		}
	}

	return seq.v.Add(^uint64(0))
}

// descending creates a sequence for a clock that runs backwards
func descending() *sequence {
	seq := &sequence{}
	seq.tick.Store(^uint64(0))
	seq.v.Store(^uint64(0))
	return seq
}

// seed sets 𝑽 to v, as if v had already been allocated, so the next call to
// next or prev continues strictly beyond it instead of from the sequence's
// zero-value start. It is ClockBuilder.WithSeed's mechanism: v is normally a
// value a prior process reported through WithCheckpoint, restoring the
// ratchet's high water mark across a restart that would otherwise drop it.
//
// seed must only be called before the sequence is handed to any allocator —
// ClockBuilder.Build is the only caller, before it returns the Chronos — so a
// plain store is enough, no concurrent access is possible yet.
func (seq *sequence) seed(v uint64) {
	seq.tick.Store(v)
	seq.v.Store(v)
}

// Process wide ⟨𝒕,𝒔⟩ sequences. Every clock of the same time domain shares one,
// so that values allocated by distinct instances of Chronos within the process
// remain unique and ordered.
var (
	seqAscending  = &sequence{}
	seqDescending = descending()
)
