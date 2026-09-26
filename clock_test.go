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
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

func TestWithNodeID(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xfedcba98),
	)
	a := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Node(), 0xfedcba98),
	)
}

func TestWithNodeFromEnv(t *testing.T) {
	os.Setenv("CONFIG_GUID_NODE_ID", "abc@go")

	c := guid.NewClock(
		guid.WithNodeFromEnv(),
	)
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

	guid.NewClock(guid.WithNodeFromEnv())
}

func TestWithNodeRand(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeRandom(),
	)
	a := guid.NewG(c)

	it.Then(t).ShouldNot(
		it.Equal(a.Node(), 0x0),
	)
}

func TestWithClock(t *testing.T) {
	c := guid.NewClock(
		guid.WithClock(func() uint64 { return 0xfedcba98 << 16 }),
	)
	a := guid.NewG(c)

	it.Then(t).Should(
		it.Equal(a.Time(), 0xfedcba98<<16),
	)
}

func TestWithClockUnix(t *testing.T) {
	c := guid.NewClock(
		guid.WithClockUnix(),
	)
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
	c := guid.NewClock(
		guid.WithClockInverse(),
	)
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
		c := guid.NewClock(
			guid.WithClock(func() uint64 {
				n++
				// jitters forward and back across a wide range, including a
				// value (1_000_000) too small to ever move the tick forward
				if n%3 == 0 {
					return 1_000_000
				}
				return n * 1_000_000_000
			}),
		)

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
		c := guid.NewClock(
			guid.WithClock(func() uint64 { return 42 }), // frozen, forces ⟨𝒔⟩ carry-over
		)

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
	c := guid.NewClockMock(
		guid.WithNodeID(0x0),
	)
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
