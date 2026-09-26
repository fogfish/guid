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
	"math/big"
	"math/rand"
	"regexp"
	"sort"
	"testing"
	"time"
	"unsafe"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

// the widest ⟨𝒍⟩ the schema admits, 58 bits
const maskNodeX = 1<<58 - 1

// The memory layout is the contract of the type: X is exactly 128 bits and
// carries no padding, so that it costs no more than the UUID it is.
func TestMemoryLayoutX(t *testing.T) {
	it.Then(t).Should(
		it.Equal(unsafe.Sizeof(guid.X{}), 16),
		it.Equal(unsafe.Sizeof([4]guid.X{}), 64),
		it.Equal(unsafe.Alignof(guid.X{}), 1),
		it.Equal(guid.SizeX, 16),
		it.Equal(guid.SizeStringX, 36),
	)
}

func TestZeroX(t *testing.T) {
	a := guid.ZeroX(guid.Clock)
	b := guid.ZeroX(guid.Clock)

	it.Then(t).Should(
		it.Equal(a, b),
		it.True(a.Equal(b)),
		it.Equal(a.Seq(), 0),
		it.Equal(a.Time(), 0),
		it.Equal(a.Node(), 0),
	).ShouldNot(
		it.True(a.Before(b)),
		it.True(a.After(b)),
		it.True(a.Equal(guid.NewX(guid.Clock))),
	)
}

// the zero value precedes everything the clock allocates, even though its
// bytes are not zero — the version and variant are present in every value
func TestZeroXPrecedes(t *testing.T) {
	for _, drift := range drifts {
		c := guid.Clock.WithDrift(drift).WithNodeID(maskNodeX)

		it.Then(t).Should(
			it.True(guid.ZeroX(c).Before(guid.NewX(c))),
		)
	}
}

func succeedsX(a, b guid.X) bool {
	if a.Time() == b.Time() {
		return b.Seq() == a.Seq()+1
	}
	return b.Time() > a.Time()
}

func TestNewX(t *testing.T) {
	c := guid.Clock
	a := guid.NewX(c)
	b := guid.NewX(c)

	it.Then(t).ShouldNot(
		it.Equal(a, b),
		it.True(a.After(b)),
		it.True(b.Before(a)),
	).Should(
		it.True(a.Before(b)),
		it.True(b.After(a)),
		it.Equal(a.Node(), b.Node()),
		it.True(succeedsX(a, b)),
	)
}

// the default clock allocates node identity above the 32 bits G has room for,
// which is the capacity X exists to provide
func TestNewXNodeWidth(t *testing.T) {
	const trials = 64

	wide := false
	for i := 0; i < trials; i++ {
		if guid.NewX(guid.Clock).Node()>>32 != 0 {
			wide = true
		}
	}

	it.Then(t).Should(
		it.True(wide),
	)
}

func TestSpecX(t *testing.T) {
	spec := map[uint64]uint64{
		1 << 16: 0,
		1 << 17: 1 << 17,
		1 << 24: 1 << 24,
		1 << 32: 1 << 32,
		1 << 62: 1 << 62,
	}

	for _, d := range drifts {
		for tc, expect := range spec {
			c := guid.Clock.WithDrift(d).WithNodeID(maskNodeX).WithClock(func() uint64 { return tc })
			a := guid.NewX(c)
			b := guid.NewX(c)

			it.Then(t).ShouldNot(
				it.Equal(a, b),
			).Should(
				it.True(a.Before(b)),
				it.True(b.After(a)),
				it.Equal(b.Seq()-a.Seq(), 1),
				it.Equal(a.Time(), b.Time()),
				it.Equal(a.Time(), expect),
				it.Equal(a.Node(), maskNodeX),
				it.Equal(a.Drift(), d),
			)
		}
	}
}

func TestDiffX(t *testing.T) {
	for _, drift := range drifts {
		c := guid.Clock.WithDrift(drift).WithNodeID(maskNodeX)

		a := guid.NewX(c)
		b := fromTimeX(c, a.Epoch().Add(-1*time.Hour))
		d := a.Diff(b)

		it.Then(t).Should(
			it.Equal(d.Node(), maskNodeX),
			it.Equal(d.Drift(), drift),
			it.Equal(d.Time(), a.Time()-b.Time()),
			it.Equal(d.Seq(), a.Seq()),
		)
	}
}

// Proposition 1 for X, checked differentially against a big.Int model of the
// positional form and the RFC 9562 splice, for every rung of the ladder:
//
//	⟦payload⟧ = 𝒅·2¹¹⁹ + 𝑬·2^(72+𝑫) + 𝒍·2^(14+𝑫) + 𝒙ₗ·2¹⁴ + 𝒔
//
// and the value is that payload with the version and variant spliced in at the
// positions the RFC fixes. This is the load-bearing test of the layout: the
// model is written from the schema, the implementation evaluates the Horner
// form of Guid.pack, and the two agree bit for bit or one of them is wrong.
func TestPropositionX(t *testing.T) {
	const trials = 20000

	for _, rung := range ladder {
		rnd := rand.New(rand.NewSource(int64(rung.bits)))

		for i := 0; i < trials; i++ {
			node := rnd.Uint64() & maskNodeX
			tc := rnd.Uint64()

			c := guid.Clock.WithDrift(rung.drift).WithNodeID(node).WithClock(func() uint64 { return tc })

			// the sequencer hands out 𝑽 = 𝒙·2¹⁴ + 𝒔 with ⟨𝒔⟩ counting from 1
			for seq := uint64(1); seq <= 2; seq++ {
				a := guid.NewX(c)

				it.Then(t).Should(
					it.Equiv(a.Bytes(), packX(rung.drift, node, tc, seq)),
					it.Equal(a.Time(), tc>>17<<17),
					it.Equal(a.Node(), node),
					it.Equal(a.Seq(), seq),
					it.Equal(a.Drift(), rung.drift),
				)
			}
		}
	}
}

// packX is the model: the positional form of the payload evaluated in
// arbitrary precision, permuted around the two reserved fields, then written
// out big-endian.
func packX(drift guid.Drift, node, t, seq uint64) []byte {
	d := drift.Bits()
	x := t >> 17

	p := big.NewInt(0)
	p = p.Or(p, shl(big.NewInt(int64(drift)), 119))
	p = p.Or(p, shl(new(big.Int).SetUint64(x>>d), 72+d))
	p = p.Or(p, shl(new(big.Int).SetUint64(node), 14+d))
	p = p.Or(p, shl(new(big.Int).SetUint64(x&(1<<d-1)), 14))
	p = p.Or(p, new(big.Int).SetUint64(seq))

	// counting from the most significant bit, as RFC 9562 does, payload bit 𝒑
	// lands at value bit 𝒑, 𝒑+4 and 𝒑+6 for the three runs the version and the
	// variant cut the payload into; counting up from the least significant, the
	// same permutation is a shift of 0, 2 and 6
	v := big.NewInt(0)
	v = v.Or(v, runX(p, 0, 62, 0))
	v = v.Or(v, runX(p, 62, 12, 64))
	v = v.Or(v, runX(p, 74, 48, 80))
	v = v.Or(v, shl(big.NewInt(0b1000), 76)) // version 8
	v = v.Or(v, shl(big.NewInt(0b10), 62))   // the RFC 9562 variant

	return v.FillBytes(make([]byte, guid.SizeX))
}

// runX takes the w bits of v at position from and places them at position to
func runX(v *big.Int, from, w, to uint64) *big.Int {
	r := new(big.Int).Rsh(v, uint(from))
	r = r.And(r, new(big.Int).Sub(shl(big.NewInt(1), w), big.NewInt(1)))
	return shl(r, to)
}

// X is a well-formed RFC 9562 UUID: the version nibble is 8 and the variant
// bits are 0b10, whatever the rung, the node and the clock.
func TestRFC9562(t *testing.T) {
	canonical := regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	for _, drift := range drifts {
		for i := 0; i <= 57; i++ {
			c := guid.Clock.WithDrift(drift).WithNodeID(1 << i)

			a := guid.NewX(c).Bytes()

			it.Then(t).Should(
				it.Equal(a[6]>>4, 0b1000),
				it.Equal(a[8]>>6, 0b10),
				it.True(canonical.MatchString(guid.NewX(c).String())),
			)
		}
	}
}

// the reserved fields are constants of the format, so they are transparent to
// a most-significant-first comparison: the bytes sort exactly as the payload
// does, which is what Before and bytes.Compare both rely on
func TestOrderTransparency(t *testing.T) {
	const trials = 4096

	for _, rung := range ladder {
		rnd := rand.New(rand.NewSource(int64(rung.bits)))

		for i := 0; i < trials; i++ {
			c := guid.Clock.WithDrift(rung.drift).WithNodeID(rnd.Uint64()).WithClock(rnd.Uint64)

			a, b := guid.NewX(c), guid.NewX(c)
			pa, pb := payloadOf(a), payloadOf(b)

			it.Then(t).Should(
				it.Equal(a.Before(b), pa.Cmp(pb) < 0),
				it.Equal(a.After(b), pa.Cmp(pb) > 0),
				it.Equal(bytes.Compare(a.Bytes(), b.Bytes()) < 0, pa.Cmp(pb) < 0),
			)
		}
	}
}

// payloadOf is the model of the inverse splice: the 122-bit payload of a value
func payloadOf(uid guid.X) *big.Int {
	v := new(big.Int).SetBytes(uid.Bytes())

	p := big.NewInt(0)
	p = p.Or(p, runX(v, 0, 62, 0))
	p = p.Or(p, runX(v, 64, 12, 62))
	p = p.Or(p, runX(v, 80, 48, 74))
	return p
}

func TestCodecX(t *testing.T) {
	for _, drift := range drifts {
		for i := 0; i <= 57; i++ {
			c := guid.Clock.WithDrift(drift).WithNodeID(1 << i)

			a := guid.NewX(c)

			b, err := fromBytesX(a.Bytes())
			it.Then(t).Should(
				it.Nil(err),
				it.Equal(b, a),
			)

			d, err := fromStringX(a.String())
			it.Then(t).Should(
				it.Nil(err),
				it.Equal(d, a),
			)

			x, err := fromBase62X(a.Base62())
			it.Then(t).Should(
				it.Nil(err),
				it.Equal(x, a),
			)

			it.Then(t).Should(
				it.Equal(foldX(8, a.Split(8)), a),
				it.Equal(foldX(4, a.Split(4)), a),
				it.Equal(foldX(2, a.Split(2)), a),
			)
		}
	}

	t.Run("UpperCase", func(t *testing.T) {
		a, err := fromStringX("06377F2A-0CB8-8A3F-B000-0000000003E9")
		b, erb := fromStringX("06377f2a-0cb8-8a3f-b000-0000000003e9")

		it.Then(t).Should(
			it.Nil(err),
			it.Nil(erb),
			it.Equal(a, b),
			it.Equal(a.String(), "06377f2a-0cb8-8a3f-b000-0000000003e9"),
		)
	})

	t.Run("Errors", func(t *testing.T) {
		_, eb62 := fromBase62X("......")
		_, elen := fromBase62X("11111111111111111111111111111")
		_, ebin := fromBytesX([]byte("xxxxxx"))
		_, estr := fromStringX("xxxxxx")
		// dashes in the wrong place
		_, edash := fromStringX("06377f2a0-cb8-8a3f-b000-0000000003e9")
		// not hexadecimal
		_, ehex := fromStringX("06377f2a-0cb8-8a3f-b000-00000000zzzz")
		// a v7 UUID does not carry these fractions at these positions
		_, ever := fromStringX("06377f2a-0cb8-7a3f-b000-0000000003e9")
		// the Microsoft variant, likewise
		_, evar := fromStringX("06377f2a-0cb8-8a3f-c000-0000000003e9")

		it.Then(t).ShouldNot(
			it.Nil(eb62),
			it.Nil(elen),
			it.Nil(ebin),
			it.Nil(estr),
			it.Nil(edash),
			it.Nil(ehex),
			it.Nil(ever),
			it.Nil(evar),
		)
	})
}

// FromString checks the version and variant fields RFC 9562 fixes at their
// known positions, the one guardrail against garbage input this format
// admits (G and L carry no such marker, see FromBytes and FromBase62 on
// those types). FromBytes and FromBase62 build the very same in-memory value
// and must apply the same guardrail, or a corrupted or foreign payload that
// happens to decode is accepted as if NewX had produced it.
func TestFromBytesAndFromBase62XRejectWrongVersion(t *testing.T) {
	c := guid.Clock
	a := guid.NewX(c)

	corrupted := a.Bytes()
	// flip the version nibble from 8 (verX) to 7, leaving the variant intact
	corrupted[6] = (corrupted[6] &^ 0xf0) | (0x7 << 4)

	var viaBytes guid.X
	copy(viaBytes[:], corrupted)

	_, ebin := fromBytesX(corrupted)
	_, eb62 := fromBase62X(viaBytes.Base62())

	it.Then(t).ShouldNot(
		it.Nil(ebin),
		it.Nil(eb62),
	)
}

func TestJSONCodecX(t *testing.T) {
	type Struct struct {
		ID guid.X `json:"id"`
	}

	for _, drift := range drifts {
		c := guid.Clock.WithDrift(drift).WithNodeID(maskNodeX)

		a := Struct{ID: guid.NewX(c)}

		b, err := json.Marshal(a)
		it.Then(t).Should(
			it.Nil(err),
			it.Equal(string(b), `{"id":"`+a.ID.String()+`"}`),
		)

		var d Struct
		it.Then(t).Should(
			it.Nil(json.Unmarshal(b, &d)),
			it.Equal(d.ID, a.ID),
		)
	}

	t.Run("Errors", func(t *testing.T) {
		var a guid.X
		it.Then(t).ShouldNot(
			it.Nil(a.UnmarshalJSON([]byte(`"xxx"`))),
			it.Nil(a.UnmarshalJSON([]byte(`1`))),
		)
	})
}

// the canonical UUID string is a fixed-width big-endian numeral, so it sorts
// the way the value does
func TestLexSortingX(t *testing.T) {
	c := guid.Clock.WithNodeRandom()

	seq := make([]guid.X, 0, 1000)
	str := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		x := guid.NewX(c)
		seq = append(seq, x)
		str = append(str, x.String())
	}

	sort.Strings(str)
	for i, x := range seq {
		it.Then(t).Should(
			it.Equal(str[i], x.String()),
		)
	}
}

// an external index that sorts the stored bytes orders the values correctly
// without decoding them
func TestMemcmpOrderingX(t *testing.T) {
	c := guid.Clock.WithNodeRandom()

	seq := make([]guid.X, 0, 1000)
	for i := 0; i < 1000; i++ {
		seq = append(seq, guid.NewX(c))
	}

	for i := 1; i < len(seq); i++ {
		it.Then(t).Should(
			it.True(seq[i-1].Before(seq[i])),
			it.True(bytes.Compare(seq[i-1].Bytes(), seq[i].Bytes()) < 0),
		)
	}
}

// ⟨𝒅⟩ is the most significant fraction of X too, so values allocated with
// different drift segregate rather than interleave
func TestDriftSegregatesX(t *testing.T) {
	seq := make([]guid.X, 0, len(drifts))
	for _, drift := range drifts {
		c := guid.Clock.WithDrift(drift).WithNodeID(maskNodeX)
		seq = append(seq, guid.NewX(c))
	}

	for i := 1; i < len(seq); i++ {
		it.Then(t).Should(
			it.True(seq[i-1].Before(seq[i])),
		)
	}
}

func TestCastX(t *testing.T) {
	for _, drift := range drifts {
		c := guid.Clock.WithDrift(drift).WithNodeID(0xffffffff)

		a := guid.NewX(c)
		g, err := gFromX(a)
		l := lFromX(a)

		it.Then(t).Should(
			it.Nil(err),
			it.Equal(g.Time(), a.Time()),
			it.Equal(g.Seq(), a.Seq()),
			it.Equal(g.Node(), a.Node()),
			it.Equal(g.Drift(), a.Drift()),
			it.Equal(l.Time(), a.Time()),
			it.Equal(l.Seq(), a.Seq()),
			it.Equal(l.Drift(), a.Drift()),
			// the node fits 32 bits here, so the cast round-trips
			it.Equal(xFromG(g), a),
			it.Equal(xFromL(c, l), a),
		)
	}
}

// A node identity wider than 32 bits cannot be represented by G: two distinct
// X nodes that differ only above bit 32, e.g. 1 and 0x100000001, would
// otherwise truncate onto the same G node and collide. FromX refuses the cast
// with an error instead, leaving the resolution to the application.
func TestCastXNodeOverflow(t *testing.T) {
	c := guid.Clock.WithNodeID(maskNodeX)

	a := guid.NewX(c)
	g, err := gFromX(a)

	it.Then(t).Should(
		it.Equal(a.Node(), maskNodeX),
		it.Equal(g, guid.G{}),
	).ShouldNot(
		it.Nil(err),
	)
}

// The exact collision the guardrail exists for: two X values whose node
// identities differ only above bit 32 must not silently cast to the same G.
func TestCastXNodeOverflowCollision(t *testing.T) {
	c1 := guid.Clock.WithNodeID(1)
	c2 := guid.Clock.WithNodeID(0x100000001)

	a := guid.NewX(c1)
	b := guid.NewX(c2)

	_, erra := gFromX(a)
	_, errb := gFromX(b)

	it.Then(t).Should(
		it.Nil(erra),
	).ShouldNot(
		it.Nil(errb),
	)
}

func TestFromTimeX(t *testing.T) {
	for _, order := range orders {
		for _, drift := range drifts {
			c := order.clock.WithDrift(drift).WithNodeID(maskNodeX)
			n := time.Now().Round(10 * time.Millisecond)

			a := fromTimeX(c, n)

			it.Then(t).Should(
				it.Equal(a.Epoch().Round(10*time.Millisecond), n),
				it.Equal(a.Node(), maskNodeX),
				it.Equal(a.Seq(), 0),
				it.Equal(a.Drift(), drift),
			)
		}
	}
}

func TestEpochX(t *testing.T) {
	for _, order := range orders {
		for _, drift := range drifts {
			c := order.clock.WithDrift(drift)

			n := time.Now()
			a := guid.NewX(c)

			it.Then(t).Should(
				it.Less(a.Epoch().Sub(n).Abs(), time.Second),
			)
		}
	}
}

func BenchmarkX(b *testing.B) {
	b.Run("NewX", func(b *testing.B) {
		var val guid.X
		for i := 0; i < b.N; i++ {
			val = guid.NewX(guid.Clock)
		}
		_ = val
	})

	b.Run("String", func(b *testing.B) {
		uid := guid.NewX(guid.Clock)
		var val string
		for i := 0; i < b.N; i++ {
			val = uid.String()
		}
		_ = val
	})

	b.Run("FromString", func(b *testing.B) {
		str := guid.NewX(guid.Clock).String()
		var val guid.X
		for i := 0; i < b.N; i++ {
			val, _ = fromStringX(str)
		}
		_ = val
	})

	b.Run("Time", func(b *testing.B) {
		uid := guid.NewX(guid.Clock)
		var val uint64
		for i := 0; i < b.N; i++ {
			val = uid.Time()
		}
		_ = val
	})

	b.Run("Node", func(b *testing.B) {
		uid := guid.NewX(guid.Clock)
		var val uint64
		for i := 0; i < b.N; i++ {
			val = uid.Node()
		}
		_ = val
	})
}
