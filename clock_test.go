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
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

func TestWithNodeID(t *testing.T) {
	c := guid.Clock.WithNodeID(0xfedcba98)
	a := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Node(), 0xfedcba98),
	)
}

func TestWithNodeFromEnv(t *testing.T) {
	os.Setenv("CONFIG_GUID_NODE_ID", "abc@go")

	c := guid.Clock.WithNodeFromEnv()
	a := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Node(), 0x53051caf),
	)
}

// A missing CONFIG_GUID_NODE_ID must not silently fall back to SHA256(""), a
// fixed node identity every such process would share, defeating the
// coordination-free uniqueness the node fraction exists for.
func TestWithNodeFromEnvRequiresVariable(t *testing.T) {
	os.Unsetenv("CONFIG_GUID_NODE_ID")

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected WithNodeFromEnv to panic when CONFIG_GUID_NODE_ID is not set")
		}
	}()

	guid.Clock.WithNodeFromEnv()
}

// Clock and Unclock take ⟨𝒍⟩ from CONFIG_GUID_NODE_ID when it is set,
// falling back to a random one otherwise -- see defaultNode. Both globals
// are initialized once, at process start, so an in-process test cannot
// observe this: by the time a test body runs and could set the variable,
// guid.Clock already has its node. This re-executes the test binary in a
// subprocess with the variable set instead, and checks Clock's node from
// inside it against the same hash TestWithNodeFromEnv already pins down for
// the same input.
func TestDefaultNodeFromEnv(t *testing.T) {
	const want = 0x53051caf

	if os.Getenv("GUID_TEST_SUBPROCESS") == "1" {
		// Node() on the Chronos itself is the full 58-bit ⟨𝒍⟩ X carries; G
		// truncates it to 32 bits, which is what TestWithNodeFromEnv checks
		// this same input against -- go through G here for the same value.
		if node := guid.NewG(guid.Clock).Node(); node != want {
			fmt.Fprintf(os.Stderr, "Clock: got node %#x, want %#x\n", node, uint64(want))
			os.Exit(1)
		}
		if node := guid.NewG(guid.Unclock).Node(); node != want {
			fmt.Fprintf(os.Stderr, "Unclock: got node %#x, want %#x\n", node, uint64(want))
			os.Exit(1)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestDefaultNodeFromEnv$")
	cmd.Env = append(os.Environ(),
		"GUID_TEST_SUBPROCESS=1",
		"CONFIG_GUID_NODE_ID=abc@go",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
}

func TestWithNodeRand(t *testing.T) {
	c := guid.Clock.WithNodeRandom()
	a := guid.NewG(c)

	it.Then(t).ShouldNot(
		it.Equal(a.Node(), 0x0),
	)
}

func TestWithClock(t *testing.T) {
	c := guid.Clock.WithClock(func() uint64 { return 0xfedcba98 << 16 })
	a := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Time(), 0xfedcba98<<16),
	)
}

func TestWithClockUnix(t *testing.T) {
	c := guid.Clock
	a := guid.NewG(c)
	b := guid.NewG(c)
	time.Sleep(2 * time.Second)
	d := guid.NewG(c)

	it.Then(t).Should(
		it.True(a.Before(b)),
		it.True(b.Before(d)),
	)
}

func TestWithClockInverse(t *testing.T) {
	c := guid.Unclock
	a := guid.NewG(c)
	b := guid.NewG(c)
	time.Sleep(2 * time.Second)
	d := guid.NewG(c)

	it.Then(t).Should(
		it.True(a.After(b)),
		it.True(b.After(d)),
	)
}

// WithClock's own doc used to ask for a non-decreasing generator without
// saying what actually depends on it. It does not: the coupled ⟨𝒕,𝒔⟩ sequence
// (Algorithm 1, doc/proof.md §3.2) only ever raises or holds, never lowers, so
// L allocated from one WithClock-configured Chronos is strictly increasing
// whatever the generator returns — including a generator that goes backwards,
// repeats, or never moves. This pins that guarantee down as a test, single
// threaded and under -race with concurrent allocators, so a future change to
// sequence.go that broke it would be caught here rather than in the field.
func TestWithClockMonotonicRegardlessOfGenerator(t *testing.T) {
	t.Run("SingleThreaded", func(t *testing.T) {
		var n uint64
		c := guid.Clock.WithClock(func() uint64 {
			n++
			// jitters forward and back across a wide range, including a
			// value (1_000_000) too small to ever move the tick forward
			if n%3 == 0 {
				return 1_000_000
			}
			return n * 1_000_000_000
		})

		prev := guid.ZeroL(c)
		for i := 0; i < 20000; i++ {
			v := guid.NewL(c)
			if !v.After(prev) {
				t.Fatalf("monotony violated at call %d: prev=%d v=%d", i, prev, v)
			}
			prev = v
		}
	})

	t.Run("Concurrent", func(t *testing.T) {
		// frozen ticker, forces ⟨𝒔⟩ carry-over
		c := guid.Clock.WithClock(func() uint64 { return 42 })

		const workers = 16
		const perWorker = 20000
		streams := make([][]guid.L, workers)
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			w := w
			streams[w] = make([]guid.L, perWorker)
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					streams[w][i] = guid.NewL(c)
				}
			}()
		}
		wg.Wait()

		seen := make(map[guid.L]bool, workers*perWorker)
		for _, s := range streams {
			for i, v := range s {
				if seen[v] {
					t.Fatalf("duplicate value: %d", v)
				}
				seen[v] = true
				if i > 0 && !s[i].After(s[i-1]) {
					t.Fatalf("worker stream not monotone at %d: prev=%d v=%d", i, s[i-1], s[i])
				}
			}
		}
	})
}

func TestWithMock(t *testing.T) {
	c := guid.Mock.WithNodeID(0x0)
	a := guid.NewG(c)
	b := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Node(), 0),
		it.Equal(a.Time(), 0),
		it.Equal(a.Seq(), 0),
		// the mock is deterministic, it repeats the very same value
		it.Equal(a, b),
		it.Equal(guid.NewL(c), guid.NewL(c)),
	)
}

// WithCheckpoint delivers the sequence's high water mark on genuine forward
// progress. The ticker is advanced by more than one tick's worth (2¹⁷ ns) on
// every allocation so every call is guaranteed to tick, and the interval is
// zero so nothing is left throttled out of the test's short run.
func TestWithCheckpointDeliversHighWaterMark(t *testing.T) {
	var now uint64 = 1 << 40
	ch := make(chan uint64, 1)

	c := guid.Clock.WithNodeID(0x1).WithClock(func() uint64 { return atomic.LoadUint64(&now) }).WithCheckpoint(0, ch)

	var last uint64
	for i := 0; i < 100; i++ {
		atomic.AddUint64(&now, 1<<17)
		guid.NewL(c)

		select {
		case v := <-ch:
			last = v
		default:
		}
	}

	it.Then(t).ShouldNot(
		it.Equal(last, uint64(0)),
	)
}

// WithCheckpoint's send never blocks the allocator: a channel nobody drains
// must not stall T, only cause the checkpoint to be missed.
func TestWithCheckpointNeverBlocksAllocation(t *testing.T) {
	var now uint64 = 1 << 40
	ch := make(chan uint64) // unbuffered and never drained

	c := guid.Clock.WithClock(func() uint64 { return atomic.LoadUint64(&now) }).WithCheckpoint(0, ch)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			atomic.AddUint64(&now, 1<<17)
			guid.NewL(c)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("allocation blocked on an undrained checkpoint channel")
	}
}

// TestWithSeedPreventsRestartRegression reproduces the hazard WithSeed and
// WithCheckpoint exist to close: a process restart that coincides with the
// wall clock reading behind where the previous process left off (an NTP
// step correction, most often) drops the in-memory ⟨𝒕,𝒔⟩ ratchet, so the
// restarted process can allocate values that sort before ones the previous
// process already handed out. Seeding the new sequence from the last
// checkpoint closes exactly that gap.
func TestWithSeedPreventsRestartRegression(t *testing.T) {
	var now uint64 = 1 << 40
	ch := make(chan uint64, 1)

	before := guid.Clock.WithNodeID(0x1).WithClock(func() uint64 { return atomic.LoadUint64(&now) }).WithCheckpoint(0, ch)

	var lastBefore guid.L
	var checkpoint uint64
	for i := 0; i < 100; i++ {
		atomic.AddUint64(&now, 1<<17)
		lastBefore = guid.NewL(before)

		select {
		case v := <-ch:
			checkpoint = v
		default:
		}
	}
	it.Then(t).ShouldNot(it.Equal(checkpoint, uint64(0)))

	// the restarted process' clock reads a minute behind where "before" left
	// off, as if NTP had just stepped it back across the restart
	restarted := atomic.LoadUint64(&now) - uint64(60*time.Second)

	withoutSeed := guid.Clock.WithNodeID(0x1).WithClock(func() uint64 { return restarted })
	afterNoSeed := guid.NewL(withoutSeed)

	withSeed := guid.Clock.WithNodeID(0x1).WithClock(func() uint64 { return restarted }).WithSeed(checkpoint)
	afterSeed := guid.NewL(withSeed)

	it.Then(t).Should(
		// unseeded, the restart regresses behind the previous process' last
		// value -- the hazard being fixed
		it.True(lastBefore.After(afterNoSeed)),
		// seeded from the checkpoint, it never does
		it.True(afterSeed.After(lastBefore)),
	)
}

// WithSeed and WithCheckpoint are local to the builder that requested them:
// they must never reach into the process-wide sequence WithClockUnix and
// WithClockInverse otherwise share, or seeding one clock would silently move
// the floor every other default-built clock in the process allocates from.
func TestWithSeedIsolatedFromSharedSequence(t *testing.T) {
	// a seed near the top of the ⟨𝒕,𝒔⟩ range: if it ever reached the shared
	// sequence, every other WithClockUnix clock in the process would be
	// poisoned by it and report a wildly wrong Epoch.
	poisoned := guid.Clock.WithSeed(^uint64(0) >> 1)
	_ = guid.NewL(poisoned)

	c := guid.Clock
	a := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Epoch().Round(time.Minute), time.Now().Round(time.Minute)),
	)
}

// WithSeed and WithCheckpoint resolve their private sequence lazily, on
// first T(), from everything accumulated on the fork chain -- not eagerly
// at each WithXXX call. That is what lets the two compose regardless of
// which is called first: neither call touches the sequence itself, so the
// one called second cannot discard what the first one set up.
func TestWithSeedAndWithCheckpointComposeRegardlessOfOrder(t *testing.T) {
	var now uint64 = 1 << 40
	const seed = uint64(1) << 50
	ticker := func() uint64 { return atomic.LoadUint64(&now) }

	ch1 := make(chan uint64, 1)
	seedThenCheckpoint := guid.Clock.WithClock(ticker).WithSeed(seed).WithCheckpoint(0, ch1)

	ch2 := make(chan uint64, 1)
	checkpointThenSeed := guid.Clock.WithClock(ticker).WithCheckpoint(0, ch2).WithSeed(seed)

	unseeded := guid.NewL(guid.Clock.WithClock(ticker))
	a := guid.NewL(seedThenCheckpoint)
	b := guid.NewL(checkpointThenSeed)

	it.Then(t).Should(
		// both resumed above the seed, not from zero -- the seed was not
		// silently discarded by the WithCheckpoint call that followed it
		it.True(a.After(unseeded)),
		it.True(b.After(unseeded)),
	)

	select {
	case <-ch1:
	default:
		t.Fatal("WithSeed(seed).WithCheckpoint(...): checkpoint did not fire")
	}
	select {
	case <-ch2:
	default:
		t.Fatal("WithCheckpoint(...).WithSeed(seed): checkpoint did not fire")
	}
}
