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
	"encoding"
	"encoding/gob"
	"testing"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

// The three types satisfy the stdlib codec interfaces, which is what makes
// them travel through gob, yaml, toml and anything else that speaks them
// without the codec knowing this package exists. The assertion is the
// compiler's: the conversions below do not build unless the method sets match.
var (
	_ encoding.TextMarshaler = guid.G{}
	_ encoding.TextMarshaler = guid.L(0)
	_ encoding.TextMarshaler = guid.X{}

	_ encoding.BinaryMarshaler = guid.G{}
	_ encoding.BinaryMarshaler = guid.L(0)
	_ encoding.BinaryMarshaler = guid.X{}

	_ encoding.TextUnmarshaler = (*guid.G)(nil)
	_ encoding.TextUnmarshaler = (*guid.L)(nil)
	_ encoding.TextUnmarshaler = (*guid.X)(nil)

	_ encoding.BinaryUnmarshaler = (*guid.G)(nil)
	_ encoding.BinaryUnmarshaler = (*guid.L)(nil)
	_ encoding.BinaryUnmarshaler = (*guid.X)(nil)
)

func TestCodecText(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(guid.WithDrift(drift), guid.WithClockUnix())

		g, l, x := guid.NewG(c), guid.NewL(c), guid.NewX(c)

		tg, erg := g.MarshalText()
		tl, erl := l.MarshalText()
		tx, erx := x.MarshalText()

		var dg guid.G
		var dl guid.L
		var dx guid.X

		it.Then(t).Should(
			it.Nil(erg), it.Nil(erl), it.Nil(erx),
			it.Equal(string(tg), g.String()),
			it.Equal(string(tl), l.String()),
			it.Equal(string(tx), x.String()),
			it.Nil(dg.UnmarshalText(tg)),
			it.Nil(dl.UnmarshalText(tl)),
			it.Nil(dx.UnmarshalText(tx)),
			it.Equal(dg, g),
			it.Equal(dl, l),
			it.Equal(dx, x),
		)
	}
}

func TestCodecBinary(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(guid.WithDrift(drift), guid.WithClockUnix())

		g, l, x := guid.NewG(c), guid.NewL(c), guid.NewX(c)

		bg, erg := g.MarshalBinary()
		bl, erl := l.MarshalBinary()
		bx, erx := x.MarshalBinary()

		var dg guid.G
		var dl guid.L
		var dx guid.X

		it.Then(t).Should(
			it.Nil(erg), it.Nil(erl), it.Nil(erx),
			it.Equiv(bg, g.Bytes()),
			it.Equiv(bl, l.Bytes()),
			it.Equiv(bx, x.Bytes()),
			it.Nil(dg.UnmarshalBinary(bg)),
			it.Nil(dl.UnmarshalBinary(bl)),
			it.Nil(dx.UnmarshalBinary(bx)),
			it.Equal(dg, g),
			it.Equal(dl, l),
			it.Equal(dx, x),
		)
	}
}

// The point of the stdlib interfaces: a codec that has never heard of this
// package round-trips the values because the method sets are the contract.
func TestCodecGob(t *testing.T) {
	type Record struct {
		G guid.G
		L guid.L
		X guid.X
	}

	c := guid.NewClock(guid.WithClockUnix())
	a := Record{G: guid.NewG(c), L: guid.NewL(c), X: guid.NewX(c)}

	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(a)

	var b Record
	ber := gob.NewDecoder(&buf).Decode(&b)

	it.Then(t).Should(
		it.Nil(err),
		it.Nil(ber),
		it.Equal(b.G, a.G),
		it.Equal(b.L, a.L),
		it.Equal(b.X, a.X),
	)
}

// A failed decode leaves the destination as it was. The decoders build in
// place, so this is the property that makes a discarded error recoverable
// rather than silently corrupting the value that was already there.
func TestDecodeKeepsDestinationOnError(t *testing.T) {
	c := guid.NewClock(guid.WithNodeID(0xffffffff), guid.WithClockUnix())

	g, l, x := guid.NewG(c), guid.NewL(c), guid.NewX(c)
	wg, wl, wx := g, l, x

	it.Then(t).ShouldNot(
		it.Nil(wg.FromString("xxxxxx")),
		it.Nil(wg.FromBytes([]byte("xx"))),
		it.Nil(wg.FromBase62("......")),
		it.Nil(wl.FromString("xxxxxx")),
		it.Nil(wl.FromBytes([]byte("xx"))),
		it.Nil(wl.FromBase62("......")),
		it.Nil(wx.FromString("xxxxxx")),
		// a well-formed UUID of the wrong version
		it.Nil(wx.FromString("06377f2a-0cb8-7a3f-b000-0000000003e9")),
		it.Nil(wx.FromBytes([]byte("xx"))),
		it.Nil(wx.FromBase62("......")),
	).Should(
		it.Equal(wg, g),
		it.Equal(wl, l),
		it.Equal(wx, x),
	)
}

// The shape the API is built for: the destination is a field of something that
// already exists, so decoding is a statement and no temporary is needed.
func TestDecodeInPlace(t *testing.T) {
	type Record struct {
		ID     guid.X
		Parent guid.G
		Seq    guid.L
	}

	c := guid.NewClock(guid.WithNodeID(0xffffffff), guid.WithClockUnix())
	x, g, l := guid.NewX(c), guid.NewG(c), guid.NewL(c)

	var row Record
	it.Then(t).Should(
		it.Nil(row.ID.FromString(x.String())),
		it.Nil(row.Parent.FromString(g.String())),
		it.Nil(row.Seq.FromString(l.String())),
		it.Equal(row.ID, x),
		it.Equal(row.Parent, g),
		it.Equal(row.Seq, l),
	)

	// and through a slice element, which is addressable too
	rows := make([]Record, 1)
	it.Then(t).Should(
		it.Nil(rows[0].ID.FromBytes(x.Bytes())),
		it.Equal(rows[0].ID, x),
	)
}
