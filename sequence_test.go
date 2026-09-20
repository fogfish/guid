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

package guid_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

// allocations per ⟨𝒔⟩ cycle
const cycle = 1 << 14

// TestSeqCarry allocates well past a ⟨𝒔⟩ cycle while ⟨𝒕⟩ stands still.
//
// ⟨𝒔⟩ is 14 bits and ⟨𝒕⟩ advances once per 2¹⁷ nanoseconds, so an allocator
// that keeps ⟨𝒔⟩ as a free running counter folds it back to 0 within the tick
// and allocates a value that sorts before its predecessor. The sequence carries
// into the next tick instead.
func TestSeqCarry(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0x1),
		guid.WithClock(func() uint64 { return 1 << 20 }),
	)

	seen := make(map[guid.L]bool, 3*cycle)
	last := guid.NewL(c)
	seen[last] = true
	inv, dup, carried := 0, 0, 0

	for i := 0; i < 3*cycle; i++ {
		uid := guid.NewL(c)

		if !last.Before(uid) {
			inv++
		}
		if seen[uid] {
			dup++
		}
		if uid.Time() != last.Time() {
			carried++
		}

		seen[uid] = true
		last = uid
	}

	// three cycles of ⟨𝒔⟩ borrow three ticks of ⟨𝒕⟩ from the frozen clock
	it.Then(t).Should(
		it.Equal(inv, 0),
		it.Equal(dup, 0),
		it.Equal(carried, 3),
		it.Equal(last.Time(), 1<<20+3*(1<<17)),
	)
}

// TestSeqCarryG is TestSeqCarry for globally unique values
func TestSeqCarryG(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xffffffff),
		guid.WithClock(func() uint64 { return 1 << 20 }),
	)

	last := guid.NewG(c)
	inv, drift := 0, 0
	for i := 0; i < 2*cycle; i++ {
		uid := guid.NewG(c)

		if !last.Before(uid) {
			inv++
		}
		if uid.Node() != 0xffffffff {
			drift++
		}
		last = uid
	}

	it.Then(t).Should(
		it.Equal(inv, 0),
		it.Equal(drift, 0),
	)
}

// TestSeqBackwards allocates across a clock that is stepped backwards, as NTP
// does when it corrects a drifted host. Values stay ordered: the sequence holds
// its high water mark until real time catches up.
func TestSeqBackwards(t *testing.T) {
	now := uint64(1 << 40)
	c := guid.NewClock(
		guid.WithNodeID(0x1),
		guid.WithClock(func() uint64 { return atomic.LoadUint64(&now) }),
	)

	last := guid.NewL(c)
	inv := 0
	for i := 0; i < 1000; i++ {
		// step the clock a minute backwards, then let it tick forward again
		switch {
		case i == 500:
			atomic.StoreUint64(&now, 1<<40-uint64(60*time.Second))
		default:
			atomic.AddUint64(&now, 1<<17)
		}

		uid := guid.NewL(c)
		if !last.Before(uid) {
			inv++
		}
		last = uid
	}

	it.Then(t).Should(
		it.Equal(inv, 0),
	)
}

// TestSeqDescending is TestSeqCarry for a clock that runs backwards, where
// values allocated later sort before their predecessors.
func TestSeqDescending(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0x1),
		guid.WithClockDescending(func() uint64 { return 1 << 40 }),
	)

	last := guid.NewL(c)
	inv := 0
	for i := 0; i < 2*cycle; i++ {
		uid := guid.NewL(c)

		if !last.After(uid) {
			inv++
		}
		last = uid
	}

	it.Then(t).Should(
		it.Equal(inv, 0),
	)
}

// TestSeqInverse checks the built-in inverse clock over a live timestamp
func TestSeqInverse(t *testing.T) {
	c := guid.NewClock(guid.WithClockInverse())

	last := guid.NewL(c)
	inv := 0
	for i := 0; i < 100000; i++ {
		uid := guid.NewL(c)
		if !last.After(uid) {
			inv++
		}
		last = uid
	}

	it.Then(t).Should(
		it.Equal(inv, 0),
	)
}

// TestSeqConcurrent allocates from many goroutines at once. Values are unique
// across the process and, as seen by each goroutine, strictly increasing.
func TestSeqConcurrent(t *testing.T) {
	const n, m = 8, 100000

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		all = make(map[guid.L]bool, n*m)
		dup int
		inv int
	)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			seq := make([]guid.L, 0, m)
			for j := 0; j < m; j++ {
				seq = append(seq, guid.NewL(guid.Clock))
			}

			mu.Lock()
			defer mu.Unlock()

			for j := 1; j < len(seq); j++ {
				if !seq[j-1].Before(seq[j]) {
					inv++
				}
			}
			for _, uid := range seq {
				if all[uid] {
					dup++
				}
				all[uid] = true
			}
		}()
	}
	wg.Wait()

	it.Then(t).Should(
		it.Equal(inv, 0),
		it.Equal(dup, 0),
		it.Equal(len(all), n*m),
	)
}

// TestSeqShared checks that clocks of one time domain share a sequence, so that
// values allocated through distinct instances of Chronos do not collide.
func TestSeqShared(t *testing.T) {
	a := guid.NewClock(guid.WithClockUnix())
	b := guid.NewClock(guid.WithClockUnix())

	it.Then(t).ShouldNot(
		it.Equal(guid.NewL(a), guid.NewL(b)),
		it.Equal(guid.NewL(b), guid.NewL(a)),
	)
}

func BenchmarkL(b *testing.B) {
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			_ = guid.NewL(guid.Clock)
		}
	})
}

func BenchmarkG(b *testing.B) {
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			_ = guid.NewG(guid.Clock)
		}
	})
}
