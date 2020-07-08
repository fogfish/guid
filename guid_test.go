//
//   Copyright 2012 Dmitry Kolesnikov, All Rights Reserved
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.
//

package guid_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fogfish/guid"
	"github.com/fogfish/it"
)

var drifts []time.Duration = []time.Duration{
	30 * time.Second,
	60 * time.Second,
	130 * time.Second,
	270 * time.Second,
	540 * time.Second,
	1000 * time.Second,
	2100 * time.Second,
	3600 * time.Second,
}

func TestZ(t *testing.T) {
	a := guid.Seq.Z()
	b := guid.Seq.Z()

	it.Ok(t).
		If(a.Eq(b)).Should().Equal(true).
		If(a.Lt(b)).ShouldNot().Equal(true).
		If(a.Time()).Should().Equal(int64(0)).
		If(a.Seq()).Should().Equal(uint64(0))
}

func TestL(t *testing.T) {
	a := guid.Seq.L()
	b := guid.Seq.L()

	it.Ok(t).
		If(a.Eq(b)).ShouldNot().Equal(true).
		If(a.Lt(b)).Should().Equal(true).
		If(b.Seq() - a.Seq()).Should().Equal(uint64(1))
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
			n := guid.New(
				guid.Allocator(0xffffffff),
				guid.Clock(func() uint64 { return tc }),
			)
			a := n.L(d)
			b := n.L(d)

			it.Ok(t).
				If(a.Eq(b)).ShouldNot().Equal(true).
				If(a.Lt(b)).Should().Equal(true).
				If(b.Seq() - a.Seq()).Should().Equal(uint64(1)).
				If(a.Time() == b.Time()).Should().Equal(true).
				If(a.Time()).Should().Equal(expect)
		}
	}
}

func TestDiffL(t *testing.T) {
	for i, drift := range drifts {
		node := guid.New(
			guid.Allocator(0xffffffff),
			guid.Clock(func() uint64 { return 1 << 17 }),
		)

		a := node.L(drift)
		b := node.L(drift)
		d := b.Diff(a)

		it.Ok(t).
			If(d.Seq()).Should().Equal(uint64(1)).
			If(d.Time()).Should().Equal(int64(0)).
			If(d.Bytes()).Should().Equal([]byte{byte(i << 5), 0, 0, 0, 0, 0, 0, 1})
	}
}

func TestDiffLZ(t *testing.T) {
	for _, drift := range drifts {
		node := guid.New(
			guid.Allocator(0xffffffff),
			guid.Clock(func() uint64 { return 1 << 17 }),
		)

		z := node.Z(drift)
		a := node.L(drift)
		d := a.Diff(z)

		it.Ok(t).
			If(d.Seq()).Should().Equal(a.Seq()).
			If(d.Time()).Should().Equal(a.Time())
	}
}

func TestG(t *testing.T) {
	a := guid.Seq.G()
	b := guid.Seq.G()

	it.Ok(t).
		If(a.Eq(b)).ShouldNot().Equal(true).
		If(a.Lt(b)).Should().Equal(true).
		If(a.Node()).Should().Equal(b.Node()).
		If(b.Seq() - a.Seq()).Should().Equal(uint64(1))
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
			n := guid.New(
				guid.Allocator(0xffffffff),
				guid.Clock(func() uint64 { return tc }),
			)
			a := n.G(d)
			b := n.G(d)

			it.Ok(t).
				If(a.Eq(b)).ShouldNot().Equal(true).
				If(a.Lt(b)).Should().Equal(true).
				If(b.Seq() - a.Seq()).Should().Equal(uint64(1)).
				If(a.Time() == b.Time()).Should().Equal(true).
				If(a.Time()).Should().Equal(expect)
		}
	}
}

func TestDiffG(t *testing.T) {
	for i, drift := range drifts {
		node := guid.New(
			guid.Allocator(0xffffffff),
			guid.Clock(func() uint64 { return 1 << 17 }),
		)

		a := node.G(drift)
		b := node.G(drift)
		d := b.Diff(a)
		bytes := d.Bytes()

		it.Ok(t).
			If(d.Seq()).Should().Equal(uint64(1)).
			If(d.Time()).Should().Equal(int64(0)).
			If(d.Node()).Should().Equal(uint64(0xffffffff)).
			If(bytes[0]).Should().Equal(byte(i << 5)).
			If(bytes[11]).Should().Equal(byte(1))
	}
}

func TestDiffGZ(t *testing.T) {
	for _, drift := range drifts {
		node := guid.New(
			guid.Allocator(0xffffffff),
			guid.Clock(func() uint64 { return 1 << 17 }),
		)

		z := node.Z(drift).ToG(node)
		a := node.G(drift)
		d := a.Diff(z)

		it.Ok(t).
			If(d.Seq()).Should().Equal(a.Seq()).
			If(d.Time()).Should().Equal(a.Time()).
			If(d.Node()).Should().Equal(a.Node())
	}
}

func TestLtoG(t *testing.T) {
	for _, drift := range drifts {
		node := guid.New(
			guid.Allocator(0xffffffff),
			guid.Clock(func() uint64 { return 1 << 17 }),
		)
		a := guid.Seq.L(drift)
		b := a.ToG(node)

		it.Ok(t).
			If(a.Time()).Should().Equal(b.Time()).
			If(a.Seq()).Should().Equal(b.Seq()).
			If(b.Node()).Should().Equal(uint64(0xffffffff))
	}
}

func TestGtoL(t *testing.T) {
	for _, drift := range drifts {
		a := guid.Seq.G(drift)
		b := a.ToL()

		it.Ok(t).
			If(a.Time()).Should().Equal(b.Time()).
			If(a.Seq()).Should().Equal(b.Seq())
	}
}

func TestCodecG(t *testing.T) {
	for i := 0; i <= 31; i++ {
		node := guid.New(
			guid.Allocator(1<<i),
			guid.Clock(func() uint64 { return 0 }),
		)

		a := node.G()
		b := guid.G{}
		b.FromBytes(a.Bytes())

		it.Ok(t).If(a.Eq(b)).Should().Equal(true)
	}
}

func TestCodecGBytes(t *testing.T) {
	for i := 0; i <= 31; i++ {
		node := guid.New(
			guid.Allocator(1<<i),
			guid.Clock(func() uint64 { return 0 }),
		)

		a := node.G()
		b := guid.G{}
		b.FromString(a.String())

		it.Ok(t).If(a.Eq(b)).Should().Equal(true)
	}
}

func TestCodecL(t *testing.T) {
	for i := 0; i <= 31; i++ {
		node := guid.New(
			guid.Allocator(1<<i),
			guid.Clock(func() uint64 { return 0 }),
		)

		a := node.L()
		b := guid.L{}
		b.FromBytes(a.Bytes())

		it.Ok(t).If(a.Eq(b)).Should().Equal(true)
	}
}

func TestCodecLBytes(t *testing.T) {
	for i := 0; i <= 31; i++ {
		node := guid.New(
			guid.Allocator(1<<i),
			guid.Clock(func() uint64 { return 0 }),
		)

		a := node.L()
		b := guid.L{}
		b.FromString(a.String())

		it.Ok(t).If(a.Eq(b)).Should().Equal(true)
	}
}

func TestOrdChars(t *testing.T) {
	a := guid.Seq.G().String()
	b := guid.Seq.G().String()

	it.Ok(t).
		If(a).ShouldNot().Equal(b).
		If(a).Should().Be().Less(b)
}

func TestJSONCodecL(t *testing.T) {
	type MyStruct struct {
		ID guid.L `json:"id"`
	}

	val := MyStruct{guid.Seq.L()}
	b, _ := json.Marshal(val)

	var x MyStruct
	json.Unmarshal(b, &x)

	it.Ok(t).
		If(val.ID.Eq(x.ID)).Should().Equal(true)
}

func TestJSONCodecG(t *testing.T) {
	type MyStruct struct {
		ID guid.G `json:"id"`
	}

	val := MyStruct{guid.Seq.G()}
	b, _ := json.Marshal(val)

	var x MyStruct
	json.Unmarshal(b, &x)

	it.Ok(t).
		If(val.ID.Eq(x.ID)).Should().Equal(true)
}

func BenchmarkL(b *testing.B) {
	b.RunParallel(func(par *testing.PB) {
		for par.Next() {
			guid.Seq.G()
		}
	})
}
