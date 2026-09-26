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
	"bytes"
	"encoding/json"
	"sort"
	"testing"
	"time"
	"unsafe"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

// every rung of the drift ladder, in the order of the ⟨𝒅⟩ code
var drifts []guid.Drift = []guid.Drift{
	guid.Drift131us,
	guid.Drift2s,
	guid.Drift17s,
	guid.Drift68s,
	guid.Drift275s,
	guid.Drift1099s,
	guid.Drift4398s,
	guid.Drift39h,
}

// The memory layout is the contract of the types: L is exactly 64 bits and G
// is exactly 96 bits, neither carries padding.
func TestMemoryLayout(t *testing.T) {
	it.Then(t).Should(
		it.Equal(unsafe.Sizeof(guid.L(0)), 8),
		it.Equal(unsafe.Sizeof(guid.G{}), 12),
		it.Equal(unsafe.Sizeof([4]guid.L{}), 32),
		it.Equal(unsafe.Sizeof([4]guid.G{}), 48),
		it.Equal(unsafe.Alignof(guid.G{}), 1),
		it.Equal(guid.SizeL, 8),
		it.Equal(guid.SizeG, 12),
	)
}

func TestZeroG(t *testing.T) {
	a := guid.ZeroG(guid.Clock)
	b := guid.ZeroG(guid.Clock)

	it.Then(t).Should(
		it.Equal(a, b),
		it.Equal(a.Seq(), 0),
		it.Equal(a.Time(), 0),
		it.Equal(a.Node(), 0),
	).ShouldNot(
		it.True(a.Before(b)),
		it.True(a.After(b)),
	)
}

func TestZeroL(t *testing.T) {
	a := guid.ZeroL(guid.Clock)
	b := guid.ZeroL(guid.Clock)

	it.Then(t).Should(
		it.Equal(a, b),
		it.Equal(a.Seq(), 0),
		it.Equal(a.Time(), 0),
	).ShouldNot(
		it.True(a.Before(b)),
		it.True(a.After(b)),
	)
}

// consecutive ⟨𝒕,𝒔⟩ allocation: ⟨𝒔⟩ increments within a tick of ⟨𝒕⟩ and
// restarts when the clock ticks, so that the pair strictly increases.
func succeedsG(a, b guid.G) bool {
	if a.Time() == b.Time() {
		return b.Seq() == a.Seq()+1
	}
	return b.Time() > a.Time()
}

func succeedsL(a, b guid.L) bool {
	if a.Time() == b.Time() {
		return b.Seq() == a.Seq()+1
	}
	return b.Time() > a.Time()
}

func TestNewG(t *testing.T) {
	c := guid.NewClock()
	a := guid.NewG(c)
	b := guid.NewG(c)

	it.Then(t).ShouldNot(
		it.Equal(a, b),
		it.True(a.After(b)),
		it.True(b.Before(a)),
	).Should(
		it.True(a.Before(b)),
		it.True(b.After(a)),
		it.Equal(a.Node(), b.Node()),
		it.True(succeedsG(a, b)),
	)
}

func TestNewL(t *testing.T) {
	c := guid.NewClock()
	a := guid.NewL(c)
	b := guid.NewL(c)

	it.Then(t).ShouldNot(
		it.Equal(a, b),
		it.True(a.After(b)),
		it.True(b.Before(a)),
	).Should(
		it.True(a.Before(b)),
		it.True(b.After(a)),
		it.True(succeedsL(a, b)),
	)
}

func TestAfter(t *testing.T) {
	for a, b := range map[string]string{
		"NiiTRfl2BaVI1B.0": "NiiTTfl2BaVBHo8R",
		"NiiTRfl2BaVI1B.1": "NiiTTfl2BaV71R8Q",
	} {
		av, _ := fromStringG(a)
		bv, _ := fromStringG(b)

		it.Then(t).Should(
			it.True(bv.After(av)),
			it.True(av.Before(bv)),
		)
	}
}

func TestSpecG(t *testing.T) {
	spec := map[uint64]int64{
		1 << 16: 0,
		1 << 17: 1 << 17,
		1 << 24: 1 << 24,
		1 << 32: 1 << 32,
		1 << 62: 1 << 62,
	}

	for _, d := range drifts {
		for tc, expect := range spec {
			c := guid.NewClock(
				guid.WithDrift(d),
				guid.WithNodeID(0xffffffff),
				guid.WithClock(func() uint64 { return tc }),
			)
			a := guid.NewG(c)
			b := guid.NewG(c)

			it.Then(t).ShouldNot(
				it.Equal(a, b),
			).Should(
				it.True(a.Before(b)),
				it.True(b.After(a)),
				it.Equal(b.Seq()-a.Seq(), 1),
				it.Equal(a.Time(), b.Time()),
				it.Equal(a.Time(), uint64(expect)),
				it.Equal(a.Node(), 0xffffffff),
			)
		}
	}
}

func TestSpecL(t *testing.T) {
	spec := map[uint64]int64{
		1 << 16: 0,
		1 << 17: 1 << 17,
		1 << 24: 1 << 24,
		1 << 32: 1 << 32,
		1 << 62: 1 << 62,
	}

	for _, d := range drifts {
		for tc, expect := range spec {
			c := guid.NewClock(
				guid.WithDrift(d),
				guid.WithNodeID(0xffffffff),
				guid.WithClock(func() uint64 { return tc }),
			)
			a := guid.NewL(c)
			b := guid.NewL(c)

			it.Then(t).ShouldNot(
				it.Equal(a, b),
			).Should(
				it.True(a.Before(b)),
				it.True(b.After(a)),
				it.Equal(b.Seq()-a.Seq(), 1),
				it.Equal(a.Time(), b.Time()),
				it.Equal(a.Time(), uint64(expect)),
			)
		}
	}
}

func TestDiffG(t *testing.T) {
	for i, drift := range drifts {
		c := guid.NewClock(
			guid.WithDrift(drift),
			guid.WithNodeID(0xffffffff),
			guid.WithClock(func() uint64 { return 1 << 17 }),
		)

		a := guid.NewG(c)
		b := guid.NewG(c)
		d := b.Diff(a)

		it.Then(t).Should(
			it.Equal(d.Seq(), 1),
			it.Equal(d.Time(), 0),
			it.Equal(d.Node(), 0xffffffff),
			it.Equal(d[0], byte(i<<5)),
			it.Equal(d[11], 1),
		)
	}
}

func TestDiffL(t *testing.T) {
	for i, drift := range drifts {
		c := guid.NewClock(
			guid.WithDrift(drift),
			guid.WithNodeID(0xffffffff),
			guid.WithClock(func() uint64 { return 1 << 17 }),
		)

		a := guid.NewL(c)
		b := guid.NewL(c)
		d := b.Diff(a)

		it.Then(t).Should(
			it.Equal(d.Seq(), 1),
			it.Equal(d.Time(), 0),
			it.Equiv(d.Bytes(), []byte{byte(i << 5), 0, 0, 0, 0, 0, 0, 1}),
		)
	}
}

func TestDiffGZero(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(
			guid.WithDrift(drift),
			guid.WithNodeID(0xffffffff),
			guid.WithClock(func() uint64 { return 1 << 17 }),
		)

		z := guid.ZeroG(c)
		a := guid.NewG(c)
		d := a.Diff(z)

		it.Then(t).Should(
			it.True(a.Equal(d)),
			it.Equal(d.Seq(), a.Seq()),
			it.Equal(d.Time(), a.Time()),
			it.Equal(d.Node(), a.Node()),
		)
	}
}

func TestDiffLZero(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(
			guid.WithDrift(drift),
			guid.WithNodeID(0xffffffff),
			guid.WithClock(func() uint64 { return 1 << 17 }),
		)

		z := guid.ZeroL(c)
		a := guid.NewL(c)
		d := a.Diff(z)

		it.Then(t).Should(
			it.True(a.Equal(d)),
			it.Equal(d.Seq(), a.Seq()),
			it.Equal(d.Time(), a.Time()),
		)
	}
}

func TestCastGFromL(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(
			guid.WithDrift(drift),
			guid.WithNodeID(0xffffffff),
			guid.WithClockUnix(),
		)

		a := guid.NewL(c)
		b := gFromL(c, a)

		it.Then(t).Should(
			it.Equal(b.Time(), a.Time()),
			it.Equal(b.Seq(), a.Seq()),
			it.Equal(b.Node(), 0xffffffff),
		)
	}
}

func TestCastLFromG(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(
			guid.WithDrift(drift),
			guid.WithNodeID(0xffffffff),
			guid.WithClockUnix(),
		)

		a := guid.NewG(c)
		b := lFromG(a)

		it.Then(t).Should(
			it.Equal(b.Time(), a.Time()),
			it.Equal(b.Seq(), a.Seq()),
		)
	}
}

// casting is an isomorphism on the ⟨𝒕,𝒔⟩ fraction
func TestCastRoundTrip(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(
			guid.WithDrift(drift),
			guid.WithNodeID(0xffffffff),
			guid.WithClockUnix(),
		)

		a := guid.NewG(c)
		b := gFromL(c, lFromG(a))

		it.Then(t).Should(
			it.Equal(a, b),
		)
	}
}

func TestCodecG(t *testing.T) {
	for i := 0; i <= 31; i++ {
		c := guid.NewClock(
			guid.WithNodeID(1<<i),
			guid.WithClockUnix(),
		)

		a := guid.NewG(c)

		b, err := fromBytesG(a.Bytes())
		it.Then(t).Should(
			it.Nil(err),
			it.Equal(b, a),
		)

		d, err := fromStringG(a.String())
		it.Then(t).Should(
			it.Nil(err),
			it.Equal(d, a),
		)

		x, err := fromBase62G(a.Base62())
		it.Then(t).Should(
			it.Nil(err),
			it.Equal(x, a),
		)
	}

	t.Run("Errors", func(t *testing.T) {
		_, eb62 := fromBase62G("......")
		_, elen := fromBase62G("1111111111111111111111")
		_, ebin := fromBytesG([]byte("xxxxxx"))
		_, estr := fromStringG("xxxxxx")

		it.Then(t).ShouldNot(
			it.Nil(eb62),
			it.Nil(elen),
			it.Nil(ebin),
			it.Nil(estr),
		)
	})
}

func TestCodecL(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(
			guid.WithDrift(drift),
			guid.WithNodeID(0xffffffff),
			guid.WithClockUnix(),
		)

		a := guid.NewL(c)

		b, err := fromBytesL(a.Bytes())
		it.Then(t).Should(
			it.Nil(err),
			it.Equal(b, a),
		)

		d, err := fromStringL(a.String())
		it.Then(t).Should(
			it.Nil(err),
			it.Equal(d, a),
		)

		x, err := fromBase62L(a.Base62())
		it.Then(t).Should(
			it.Nil(err),
			it.Equal(x, a),
		)
	}

	t.Run("Errors", func(t *testing.T) {
		_, eb62 := fromBase62L("......")
		_, elen := fromBase62L("1111111111111111111111")
		_, ebin := fromBytesL([]byte("xxxxxx"))
		_, estr := fromStringL("xxxxxx")

		it.Then(t).ShouldNot(
			it.Nil(eb62),
			it.Nil(elen),
			it.Nil(ebin),
			it.Nil(estr),
		)
	})
}

// the clock direction is a property of the keyspace, not of the event: both
// domains are expected to round-trip the same instant through Epoch.
var orders = []struct {
	name  string
	clock guid.Config
}{
	{"ascending", guid.WithClockUnix()},
	{"descending", guid.WithClockInverse()},
}

func TestFromTimeL(t *testing.T) {
	for _, order := range orders {
		for _, drift := range drifts {
			c := guid.NewClock(guid.WithDrift(drift), order.clock)
			n := time.Now().Round(10 * time.Millisecond)

			a := fromTimeL(c, n)
			b := gFromL(c, a)

			it.Then(t).Should(
				it.Equal(a.Epoch().Round(10*time.Millisecond), n),
				it.Equal(b.Epoch().Round(10*time.Millisecond), n),
			)
		}
	}
}

func TestFromTimeG(t *testing.T) {
	for _, order := range orders {
		for _, drift := range drifts {
			c := guid.NewClock(guid.WithDrift(drift), guid.WithNodeID(0xffffffff), order.clock)
			n := time.Now().Round(10 * time.Millisecond)

			a := fromTimeG(c, n)
			v := a.Epoch().Round(10 * time.Millisecond)

			it.Then(t).Should(
				it.Equal(v, n),
				it.Equal(a.Node(), 0xffffffff),
			)
		}
	}
}

// FromT places the instant into the domain of the clock, so a value built from
// a timestamp sorts against values the same clock allocates.
func TestFromTOrder(t *testing.T) {
	for _, order := range orders {
		c := guid.NewClock(order.clock)
		past := fromTimeL(c, time.Now().Add(-time.Hour))

		a := guid.NewL(c)
		g := fromTimeG(c, time.Now().Add(-time.Hour))
		b := guid.NewG(c)

		if order.name == "ascending" {
			it.Then(t).Should(
				it.True(past.Before(a)),
				it.True(g.Before(b)),
			)
		} else {
			it.Then(t).Should(
				it.True(past.After(a)),
				it.True(g.After(b)),
			)
		}
	}
}

// Epoch recovers the domain from ⟨𝒕⟩ itself, so one method serves both an
// ascending and a descending clock without the caller naming the direction.
func TestEpoch(t *testing.T) {
	n := time.Now().Round(10 * time.Millisecond)

	for _, c := range []guid.Chronos{
		guid.NewClock(guid.WithClock(func() uint64 { return uint64(n.UnixNano()) })),
		guid.NewClock(guid.WithClockDescending(func() uint64 { return 0xffffffffffffffff - uint64(n.UnixNano()) })),
	} {
		a := guid.NewG(c)
		b := guid.NewL(c)

		it.Then(t).Should(
			it.Equal(a.Epoch().Round(10*time.Millisecond), n),
			it.Equal(b.Epoch().Round(10*time.Millisecond), n),
			it.Equal(lFromG(a).Epoch().Round(10*time.Millisecond), n),
		)
	}
}

// the two domains are separated by the top bit of ⟨𝒕⟩, exactly until the day
// int64 nanoseconds overflow. Guard the boundary the discrimination rests on.
func TestEpochDomainBoundary(t *testing.T) {
	for _, n := range []time.Time{
		time.Unix(0, 1),
		time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC),
		time.Date(2262, 4, 11, 0, 0, 0, 0, time.UTC),
	} {
		asc := guid.NewClock(guid.WithClock(func() uint64 { return uint64(n.UnixNano()) }))
		dsc := guid.NewClock(guid.WithClockDescending(func() uint64 { return 0xffffffffffffffff - uint64(n.UnixNano()) }))

		// time.Unix reports in the local zone, normalise before comparing
		it.Then(t).Should(
			it.Equal(guid.NewL(asc).Epoch().UTC().Round(time.Second), n.UTC().Round(time.Second)),
			it.Equal(guid.NewL(dsc).Epoch().UTC().Round(time.Second), n.UTC().Round(time.Second)),
		)
	}
}

func TestLexSorting(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xffffffff),
		guid.WithClockUnix(),
	)

	a := guid.NewG(c).String()
	b := guid.NewG(c).String()
	it.Then(t).ShouldNot(
		it.Equal(a, b),
	).Should(
		it.Less(a, b),
	)

	e := guid.NewL(c).String()
	f := guid.NewL(c).String()

	it.Then(t).ShouldNot(
		it.Equal(e, f),
	).Should(
		it.Less(e, f),
	)
}

func TestLexSortingBase62(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xffffffff),
		guid.WithClockUnix(),
	)

	a := guid.NewG(c).Base62()
	b := guid.NewG(c).Base62()
	it.Then(t).ShouldNot(
		it.Equal(a, b),
	).Should(
		it.Less(a, b),
	)

	e := guid.NewL(c).Base62()
	f := guid.NewL(c).Base62()

	it.Then(t).ShouldNot(
		it.Equal(e, f),
	).Should(
		it.Less(e, f),
	)
}

// Base62 is a positional numeral system: a value one digit narrower than its
// neighbor should compare smaller, but stripping the leading zero digits
// before returning the string makes it *shorter* instead, and a shorter
// string sorts before a longer one with the same prefix regardless of digit
// value. The 61 -> 62 boundary crosses from a one-digit encoding ("z") to a
// two-digit one ("10"), and "10" < "z" in ASCII even though 62 > 61: the
// numeric and lexicographic orders disagree. Base62 has to be a fixed width
// per type, zero-padded, for lexicographic order to track numeric order.
func TestBase62FixedWidthPreservesOrder(t *testing.T) {
	a := guid.L(61)
	b := guid.L(62)

	it.Then(t).Should(
		it.Equal(len(a.Base62()), len(b.Base62())),
		it.Less(a.Base62(), b.Base62()),
	)
}

// G is stored in the representation its order is defined in, memcmp over the
// raw bytes is therefore a valid comparator.
func TestMemcmpOrdering(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xffffffff),
		guid.WithClockUnix(),
	)

	seq := make([]guid.G, 1024)
	for i := range seq {
		seq[i] = guid.NewG(c)
	}

	shuffled := make([]guid.G, len(seq))
	copy(shuffled, seq)
	sort.Slice(shuffled, func(i, j int) bool {
		return bytes.Compare(shuffled[i][:], shuffled[j][:]) < 0
	})

	for i := range seq {
		it.Then(t).Should(
			it.Equal(shuffled[i], seq[i]),
			it.True(bytes.Equal(seq[i][:], seq[i].Bytes())),
		)
	}
}

func TestSplit(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xffffffff),
		guid.WithClockUnix(),
	)

	a := guid.NewG(c)
	b := guid.NewL(c)

	it.Then(t).Should(
		it.Equiv(a.Bytes(), a.Split(8)),
		it.Equiv(b.Bytes(), b.Split(8)),
		it.Equal(foldG(8, a.Split(8)), a),
		it.Equal(foldL(8, b.Split(8)), b),
	)
}

func TestJSONCodec(t *testing.T) {
	type MyStruct struct {
		G guid.G `json:"g"`
		L guid.L `json:"l"`
	}

	c := guid.NewClock(
		guid.WithNodeID(0xffffffff),
		guid.WithClockUnix(),
	)
	val := MyStruct{G: guid.NewG(c), L: guid.NewL(c)}
	b, _ := json.Marshal(val)

	var x MyStruct
	err := json.Unmarshal(b, &x)

	it.Then(t).Should(
		it.Nil(err),
		it.Equal(val.G, x.G),
		it.Equal(val.L, x.L),
	)
}

func TestJSONCodecFailed(t *testing.T) {
	type StructG struct {
		ID guid.G `json:"id"`
	}
	type StructL struct {
		ID guid.L `json:"id"`
	}

	for _, tt := range []string{
		`{"id":100}`,
		`{"id":"*****"}`,
	} {
		var g StructG
		var l StructL

		it.Then(t).ShouldNot(
			it.Nil(json.Unmarshal([]byte(tt), &g)),
			it.Nil(json.Unmarshal([]byte(tt), &l)),
		)
	}
}

var (
	gid guid.G
	lid guid.L
	str string
	bin []byte
	val uint64
)

func BenchmarkGUID(b *testing.B) {
	b.Run("NewG", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			gid = guid.NewG(guid.Clock)
		}
	})

	b.Run("NewL", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			lid = guid.NewL(guid.Clock)
		}
	})

	b.Run("String", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			str = guid.NewG(guid.Clock).String()
		}
	})

	b.Run("Bytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			bin = guid.NewG(guid.Clock).Bytes()
		}
	})

	b.Run("Time", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			val = guid.NewG(guid.Clock).Time()
		}
	})

	b.Run("Node", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			val = guid.NewG(guid.Clock).Node()
		}
	})
}
