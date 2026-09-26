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
	"slices"
	"testing"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

// The order cut at the origin is the order of the type. This is the law that
// ties the rotated comparator back to the one doc/proof.md proves: it reads
// ⟨𝒅⟩, ⟨𝑬⟩, ⟨𝒍⟩, ⟨𝒙ₗ⟩ and ⟨𝒔⟩ out of the value by hand, so anything misplaced
// in that decomposition shows up here as disagreement with Before.
func TestOrdRingAtOriginIsBefore(t *testing.T) {
	for _, drift := range drifts {
		uids := allocG(t, drift, 64)
		ord := guid.OrdRingG(0)

		for _, a := range uids {
			for _, b := range uids {
				it.Then(t).Should(
					it.Equal(ord.Before(a, b), a.Before(b)),
					it.Equal(ord.After(a, b), a.After(b)),
				)
			}
		}
	}
}

// ⟨𝒅⟩ is the most significant fraction, so values of different rungs are
// segregated rather than interleaved. The rotated comparator has to preserve
// that: it reads the rung to locate every field below it, and a corpus of one
// rung would never notice if it stopped ranking by it.
func TestOrdRingAtOriginIsBeforeMixedDrift(t *testing.T) {
	uids := []guid.G{}
	for _, drift := range drifts {
		uids = append(uids, allocG(t, drift, 8)...)
	}

	ord := guid.OrdRingG(0)
	for _, a := range uids {
		for _, b := range uids {
			it.Then(t).Should(
				it.Equal(ord.Before(a, b), a.Before(b)),
			)
		}
	}
}

func TestOrdRingAtOriginIsBeforeX(t *testing.T) {
	for _, drift := range drifts {
		uids := allocX(t, drift, 64)
		ord := guid.OrdRingX(0)

		for _, a := range uids {
			for _, b := range uids {
				it.Then(t).Should(
					it.Equal(ord.Before(a, b), a.Before(b)),
					it.Equal(ord.After(a, b), a.After(b)),
				)
			}
		}
	}
}

// The X counterpart of TestOrdRingAtOriginIsBeforeMixedDrift: ⟨𝒅⟩ segregates
// rungs in the payload the same way it does in G, and OrdRingX.Compare has
// its own early-return for a drift mismatch, so it needs its own witness.
func TestOrdRingAtOriginIsBeforeMixedDriftX(t *testing.T) {
	uids := []guid.X{}
	for _, drift := range drifts {
		uids = append(uids, allocX(t, drift, 8)...)
	}

	ord := guid.OrdRingX(0)
	for _, a := range uids {
		for _, b := range uids {
			it.Then(t).Should(
				it.Equal(ord.Before(a, b), a.Before(b)),
			)
		}
	}
}

// The point of the comparator: whoever owns the key sorts first, at every cut
// point. Values are allocated at one instant on nodes spread around the ring,
// so ⟨𝑬⟩ is shared and ⟨𝒍⟩ alone decides — the case Before gets wrong on the
// arc that wraps the origin.
func TestOrdRingPrimaryIsLeast(t *testing.T) {
	nodes := []uint64{1 << 4, 1 << 12, 1 << 20, 1 << 28}

	uids := make([]guid.G, len(nodes))
	for i, node := range nodes {
		uids[i] = guid.NewG(guid.Mock.WithNodeID(node))
	}

	// at every cut point, the least value is the first node clockwise of it
	for _, ref := range []uint64{0, 1, 1 << 4, 1<<4 + 1, 1 << 20, 1<<28 + 1} {
		ord := guid.OrdRingG(ref)

		sorted := slices.Clone(uids)
		slices.SortFunc(sorted, ord.Compare)

		it.Then(t).Should(
			it.Equal(sorted[0].Node(), successor(nodes, ref)),
		)
	}
}

// Before is right only where the successor list does not wrap the origin. The
// ring comparator is right everywhere, which is the whole claim: cutting at
// the key removes the seam rather than making it smaller.
func TestOrdRingHasNoSeam(t *testing.T) {
	nodes := []uint64{1 << 4, 1 << 12, 1 << 20, 1 << 28}

	uids := make([]guid.G, len(nodes))
	for i, node := range nodes {
		uids[i] = guid.NewG(guid.Mock.WithNodeID(node))
	}

	natural := slices.Clone(uids)
	slices.SortFunc(natural, func(a, b guid.G) int {
		switch {
		case a.Before(b):
			return -1
		case a.After(b):
			return +1
		default:
			return 0
		}
	})

	// a key past the last node wraps: its primary is the lowest token, and the
	// natural order happens to agree
	ord := guid.OrdRingG(1<<28 + 1)
	wrapped := slices.Clone(uids)
	slices.SortFunc(wrapped, ord.Compare)

	it.Then(t).Should(
		it.Equal(wrapped[0].Node(), uint64(1<<4)),
		it.Equal(natural[0].Node(), uint64(1<<4)),
	)

	// a key in the middle does not wrap, and there the two disagree
	ord = guid.OrdRingG(1 << 16)
	inner := slices.Clone(uids)
	slices.SortFunc(inner, ord.Compare)

	it.Then(t).Should(
		it.Equal(inner[0].Node(), uint64(1<<20)),
		it.Equal(natural[0].Node(), uint64(1<<4)),
	)
}

// Rotation permutes the runs of an epoch, it never splits one. This is what
// lets the index keep the natural order while conflict resolution uses the
// ring one — a scan finds the same runs either way.
func TestOrdRingKeepsRunsContiguous(t *testing.T) {
	nodes := []uint64{1 << 4, 1 << 12, 1 << 20, 1 << 28}

	// all within one epoch, so ⟨𝒍⟩ decides the grouping, but at distinct ticks
	// inside it so the values of a node are not copies of each other
	uids := make([]guid.G, 0, len(nodes)*8)
	for n, node := range nodes {
		tick := uint64(n) << bitsSeqDriftTest
		clock := guid.Clock.WithNodeID(node).WithClock(func() uint64 { return tick })
		for i := 0; i < 8; i++ {
			uids = append(uids, guid.NewG(clock))
		}
	}

	for _, ref := range []uint64{0, 1 << 8, 1 << 16, 1 << 24, 1<<28 + 7} {
		sorted := slices.Clone(uids)
		slices.SortFunc(sorted, guid.OrdRingG(ref).Compare)

		seen := map[uint64]bool{}
		for i, uid := range sorted {
			if i > 0 && sorted[i-1].Node() == uid.Node() {
				continue
			}
			it.Then(t).Should(it.Equal(seen[uid.Node()], false))
			seen[uid.Node()] = true
		}
	}
}

// ⟨𝑬⟩ outranks ⟨𝒍⟩ under the rotated order exactly as it does under Before:
// rotation reaches inside an epoch and no further.
func TestOrdRingEpochDominates(t *testing.T) {
	early := guid.NewG(guid.Mock.WithNodeID(1 << 28))
	late := guid.NewG(guid.Clock.WithNodeID(1 << 4).WithClock(func() uint64 { return 1 << 60 }))

	for _, ref := range []uint64{0, 1 << 4, 1 << 16, 1 << 28} {
		ord := guid.OrdRingG(ref)
		it.Then(t).Should(
			it.Equal(ord.Before(early, late), true),
			it.Equal(ord.Before(late, early), false),
		)
	}
}

// ref is taken at the width of ⟨𝒍⟩ and masked to it, the same truncation a
// value undergoes when WithNodeID stamps it. The two have to agree: a token
// that G narrows to 32 bits has to be compared against the same 32 bits, or
// the order is cut at a point no value can occupy.
func TestOrdRingRefIsMaskedLikeNodeID(t *testing.T) {
	const token = 0xDEADBEEF_CAFEBABE

	it.Then(t).Should(
		it.Equal(guid.OrdRingG(token).Ref(), token&(1<<32-1)),
		it.Equal(guid.OrdRingX(token).Ref(), token&(1<<58-1)),
	)

	// A ref wider than the field orders exactly as its masked form does. The
	// tokens have to straddle the masked ref — some above the cut, some below —
	// or every one of them wraps and the two forms agree for the wrong reason.
	nodes := []uint64{0x10, 0x1000, 0xC0000000, 0xFFFF0000}
	uids := make([]guid.G, len(nodes))
	for i, node := range nodes {
		uids[i] = guid.NewG(guid.Mock.WithNodeID(node))
	}

	wide := slices.Clone(uids)
	slices.SortFunc(wide, guid.OrdRingG(token).Compare)

	narrow := slices.Clone(uids)
	slices.SortFunc(narrow, guid.OrdRingG(token&(1<<32-1)).Compare)

	it.Then(t).Should(it.Seq(wide).Equal(narrow...))

	// the same at the 58 bits X gives ⟨𝒍⟩, straddling the ref masked to that
	nodesX := []uint64{0x10, 0x1000, 0x0100000000000000, 0x03F0000000000000}
	uidsX := make([]guid.X, len(nodesX))
	for i, node := range nodesX {
		uidsX[i] = guid.NewX(guid.Mock.WithNodeID(node))
	}

	wideX := slices.Clone(uidsX)
	slices.SortFunc(wideX, guid.OrdRingX(token).Compare)

	narrowX := slices.Clone(uidsX)
	slices.SortFunc(narrowX, guid.OrdRingX(token&(1<<58-1)).Compare)

	it.Then(t).Should(it.Seq(wideX).Equal(narrowX...))
}

// successor is the first node clockwise of ref, the primary the ring means.
func successor(nodes []uint64, ref uint64) uint64 {
	best, dist := uint64(0), ^uint64(0)
	for _, node := range nodes {
		if d := (node - ref) & (1<<32 - 1); d < dist {
			best, dist = node, d
		}
	}
	return best
}

// ticker spreads values over several epochs of the rung under test and over
// several ticks inside each, so that ⟨𝑬⟩ and ⟨𝒙ₗ⟩ both vary.
//
// It holds each tick for two calls and then advances, so a clock yields both
// pairs that share ⟨𝒙ₗ⟩ and are separated by ⟨𝒔⟩, and pairs that share ⟨𝒅𝑬𝒍⟩
// and are separated by ⟨𝒙ₗ⟩. Each is the only witness to the placement of one
// fraction, and a corpus without both lets a mislocated field pass.
func ticker(drift guid.Drift, i int) func() uint64 {
	base := uint64(i)<<(bitsSeqDriftTest+drift.Bits()) | uint64(i%7)<<bitsSeqDriftTest
	var call uint64

	return func() uint64 {
		call++
		return base + (call/2)<<bitsSeqDriftTest
	}
}

const bitsSeqDriftTest = 17

func allocG(t *testing.T, drift guid.Drift, n int) []guid.G {
	t.Helper()

	uids := make([]guid.G, 0, 4*n)
	for i := 0; i < n; i++ {
		clock := guid.Clock.WithDrift(drift).WithNodeID(uint64(i) * 0x9E3779B1).WithClock(ticker(drift, i))
		uids = append(uids, guid.NewG(clock), guid.NewG(clock), guid.NewG(clock), guid.NewG(clock))
	}

	assertVaries(t, len(uids),
		func(i int) (uint64, uint64, uint64) {
			return uids[i].Time(), uids[i].Node(), uids[i].Seq()
		})

	return uids
}

func allocX(t *testing.T, drift guid.Drift, n int) []guid.X {
	t.Helper()

	uids := make([]guid.X, 0, 4*n)
	for i := 0; i < n; i++ {
		clock := guid.Clock.WithDrift(drift).WithNodeID(uint64(i) * 0x9E3779B97F4A7C15).WithClock(ticker(drift, i))
		uids = append(uids, guid.NewX(clock), guid.NewX(clock), guid.NewX(clock), guid.NewX(clock))
	}

	assertVaries(t, len(uids),
		func(i int) (uint64, uint64, uint64) {
			return uids[i].Time(), uids[i].Node(), uids[i].Seq()
		})

	return uids
}

// assertVaries fails if the corpus is constant in any fraction the comparator
// reads, which would let the law below hold for the wrong reason.
func assertVaries(t *testing.T, n int, at func(int) (uint64, uint64, uint64)) {
	t.Helper()

	times, nodes, seqs := map[uint64]bool{}, map[uint64]bool{}, map[uint64]bool{}
	for i := 0; i < n; i++ {
		tm, node, seq := at(i)
		times[tm], nodes[node], seqs[seq] = true, true, true
	}

	it.Then(t).Should(
		it.Less(1, len(times)),
		it.Less(1, len(nodes)),
		it.Less(1, len(seqs)),
	)
}
