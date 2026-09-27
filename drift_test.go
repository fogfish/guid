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
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

// the ladder of doc/proof.md §1.1: the ⟨𝒅⟩ code, the 𝑫 it selects and the
// window Δ = 2^(17+𝑫) it opens
var ladder = []struct {
	drift  guid.Drift
	bits   uint64
	window time.Duration
}{
	{guid.Drift131us, 0, 131072},
	{guid.Drift2s, 14, 2147483648},
	{guid.Drift17s, 17, 17179869184},
	{guid.Drift68s, 19, 68719476736},
	{guid.Drift275s, 21, 274877906944},
	{guid.Drift1099s, 23, 1099511627776},
	{guid.Drift4398s, 25, 4398046511104},
	{guid.Drift39h, 30, 140737488355328},
}

func TestDriftLadder(t *testing.T) {
	for code, rung := range ladder {
		it.Then(t).Should(
			it.Equal(uint64(rung.drift), uint64(code)),
			it.Equal(rung.drift.Bits(), rung.bits),
			it.Equal(rung.drift.Window(), rung.window),
			it.Equal(rung.drift.Window(), time.Duration(1)<<(17+rung.bits)),
			it.Equal(rung.drift.String(), rung.window.String()),
		)
	}
}

// DriftOf selects the smallest rung whose window covers the budget, so the
// k-ordering absorbs at least as long an overlap as was asked for.
func TestDriftOf(t *testing.T) {
	for _, tc := range []struct {
		budget time.Duration
		expect guid.Drift
	}{
		{0, guid.Drift131us},
		{131 * time.Microsecond, guid.Drift131us},
		{time.Millisecond, guid.Drift2s},
		{time.Second, guid.Drift2s},
		{5 * time.Second, guid.Drift17s},
		{45 * time.Second, guid.Drift68s},
		{time.Minute, guid.Drift68s},
		// 274.9 s is just under five minutes, so the rung above it answers
		{5 * time.Minute, guid.Drift1099s},
		{10 * time.Minute, guid.Drift1099s},
		{time.Hour, guid.Drift4398s},
		{24 * time.Hour, guid.Drift39h},
		{7 * 24 * time.Hour, guid.Drift39h},
	} {
		d := guid.DriftOf(tc.budget)
		it.Then(t).Should(
			it.Equal(d, tc.expect),
			it.True(d.Window() >= tc.budget || d == guid.Drift39h),
		)
	}
}

// The drift travels with the value: it is the most significant fraction of
// both types and survives every conversion between them.
func TestDriftOfValue(t *testing.T) {
	for _, rung := range ladder {
		c := guid.NewClock(guid.Clock, guid.WithDrift(rung.drift), guid.WithNodeID(0xffffffff), guid.WithClock(func() uint64 { return 1 << 40 }))

		g := guid.NewG(c)
		l := guid.NewL(c)

		it.Then(t).Should(
			it.Equal(c.Drift(), rung.drift),
			it.Equal(g.Drift(), rung.drift),
			it.Equal(l.Drift(), rung.drift),
			it.Equal(lFromG(g).Drift(), rung.drift),
			it.Equal(gFromL(c, l).Drift(), rung.drift),
			it.Equal(guid.ZeroG(c).Drift(), rung.drift),
			it.Equal(guid.ZeroL(c).Drift(), rung.drift),
			it.Equal(g[0]>>5, byte(rung.drift)),
		)
	}
}

// The values allocated with distinct drift are segregated rather than
// interleaved — ⟨𝒅⟩ is the most significant fraction, which is assumption (A1)
// of doc/proof.md seen from the outside.
func TestDriftSegregates(t *testing.T) {
	seq := make([]guid.G, 0, len(ladder))
	for _, rung := range ladder {
		c := guid.NewClock(guid.Clock, guid.WithDrift(rung.drift), guid.WithNodeID(0xffffffff))
		seq = append(seq, guid.NewG(c))
	}

	for i := 1; i < len(seq); i++ {
		it.Then(t).Should(
			it.True(seq[i-1].Before(seq[i])),
		)
	}
}

// Proposition 1 of doc/proof.md, checked differentially against a big.Int
// model of the positional form for every rung of the ladder:
//
//	⟦makeG(𝒍, 𝑫, 𝒕, 𝒔)⟧ = 𝒅·2⁹³ + 𝑬·2^(46+𝑫) + 𝒍·2^(14+𝑫) + 𝒙ₗ·2¹⁴ + 𝒔
//
// This is the load-bearing test of the layout: it discharges the proposition
// against the implementation, including the rungs where ⟨𝑬⟩ spills across the
// boundary of the two machine words (𝑫 < 18).
func TestPropositionG(t *testing.T) {
	const trials = 20000

	for _, rung := range ladder {
		rnd := rand.New(rand.NewSource(int64(rung.bits)))

		for i := 0; i < trials; i++ {
			node := rnd.Uint64() & 0xffffffff
			tc := rnd.Uint64()

			c := guid.NewClock(guid.Clock, guid.WithDrift(rung.drift), guid.WithNodeID(node), guid.WithClock(func() uint64 { return tc }))

			// the sequencer hands out 𝑽 = 𝒙·2¹⁴ + 𝒔 with ⟨𝒔⟩ counting from 1
			for seq := uint64(1); seq <= 2; seq++ {
				a := guid.NewG(c)

				it.Then(t).Should(
					it.Equiv(a.Bytes(), packG(rung.drift, node, tc, seq)),
					it.Equal(a.Time(), tc>>17<<17),
					it.Equal(a.Node(), node),
					it.Equal(a.Seq(), seq),
				)
			}
		}
	}
}

// Proposition 2 of doc/proof.md, ⟦makeL(𝑫, 𝒕, 𝒔)⟧ = 𝒅·2⁶¹ + 𝒙·2¹⁴ + 𝒔. The
// local layout is independent of 𝑫 — there is no ⟨𝒍⟩ for time to rank against
// — but the code still travels with the value.
func TestPropositionL(t *testing.T) {
	const trials = 20000

	for _, rung := range ladder {
		rnd := rand.New(rand.NewSource(int64(rung.bits)))

		for i := 0; i < trials; i++ {
			tc := rnd.Uint64()

			c := guid.NewClock(guid.Clock, guid.WithDrift(rung.drift), guid.WithClock(func() uint64 { return tc }))

			// the sequencer hands out 𝑽 = 𝒙·2¹⁴ + 𝒔 with ⟨𝒔⟩ counting from 1
			for seq := uint64(1); seq <= 2; seq++ {
				a := guid.NewL(c)

				it.Then(t).Should(
					it.Equiv(a.Bytes(), packL(rung.drift, tc, seq)),
					it.Equal(a.Time(), tc>>17<<17),
					it.Equal(a.Seq(), seq),
				)
			}
		}
	}
}

// packG is the model: the positional form of Proposition 1 evaluated in
// arbitrary precision, then written out big-endian.
func packG(drift guid.Drift, node, t, seq uint64) []byte {
	d := drift.Bits()
	x := t >> 17

	v := big.NewInt(0)
	v = v.Or(v, shl(big.NewInt(int64(drift)), 93))
	v = v.Or(v, shl(new(big.Int).SetUint64(x>>d), 46+d))
	v = v.Or(v, shl(new(big.Int).SetUint64(node), 14+d))
	v = v.Or(v, shl(new(big.Int).SetUint64(x&(1<<d-1)), 14))
	v = v.Or(v, new(big.Int).SetUint64(seq))

	return v.FillBytes(make([]byte, guid.SizeG))
}

// packL is the model of Proposition 2.
func packL(drift guid.Drift, t, seq uint64) []byte {
	v := big.NewInt(0)
	v = v.Or(v, shl(big.NewInt(int64(drift)), 61))
	v = v.Or(v, shl(new(big.Int).SetUint64(t>>17), 14))
	v = v.Or(v, new(big.Int).SetUint64(seq))

	return v.FillBytes(make([]byte, guid.SizeL))
}

func shl(v *big.Int, n uint64) *big.Int { return v.Lsh(v, uint(n)) }
