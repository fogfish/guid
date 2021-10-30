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
	c := guid.NewLClock()
	a := guid.Z(c)
	b := guid.Z(c)

	it.Ok(t).
		If(guid.Eq(a, b)).Should().Equal(true).
		If(guid.Lt(a, b)).ShouldNot().Equal(true).
		If(guid.Seq(a)).Should().Equal(uint64(0)).
		If(guid.Time(a)).Should().Equal(uint64(0)).
		If(guid.Epoch(a)).Should().Equal(time.Unix(0, 0))
}

func TestL(t *testing.T) {
	c := guid.NewLClock()
	a := guid.L(c)
	b := guid.L(c)

	it.Ok(t).
		If(guid.Eq(a, b)).ShouldNot().Equal(true).
		If(guid.Lt(a, b)).Should().Equal(true).
		If(guid.Lt(b, a)).Should().Equal(false).
		If(guid.Seq(b) - guid.Seq(a)).Should().Equal(uint64(1))
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
			c := guid.NewLClock(
				guid.NodeID(0xffffffff),
				guid.Clock(func() uint64 { return tc }),
			)
			a := guid.L(c, d)
			b := guid.L(c, d)

			it.Ok(t).
				If(guid.Eq(a, b)).ShouldNot().Equal(true).
				If(guid.Lt(a, b)).Should().Equal(true).
				If(guid.Seq(b) - guid.Seq(a)).Should().Equal(uint64(1)).
				If(guid.Time(a) == guid.Time(b)).Should().Equal(true).
				If(guid.Time(a)).Should().Equal(uint64(expect))
		}
	}
}

func TestDiffL(t *testing.T) {
	for i, drift := range drifts {
		c := guid.NewLClock(
			guid.NodeID(0xffffffff),
			guid.Clock(func() uint64 { return 1 << 17 }),
		)

		a := guid.L(c, drift)
		b := guid.L(c, drift)
		d := guid.Diff(b, a)

		it.Ok(t).
			If(guid.Seq(d)).Should().Equal(uint64(1)).
			If(guid.Time(d)).Should().Equal(uint64(0)).
			If(guid.Bytes(d)).Should().Equal([]byte{byte(i << 5), 0, 0, 0, 0, 0, 0, 1})
	}
}

func TestDiffLZ(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewLClock(
			guid.NodeID(0xffffffff),
			guid.Clock(func() uint64 { return 1 << 17 }),
		)

		z := guid.Z(c, drift)
		a := guid.L(c, drift)
		d := guid.Diff(a, z)

		it.Ok(t).
			If(guid.Eq(d, a)).Should().Equal(true).
			If(guid.Seq(d)).Should().Equal(guid.Seq(a)).
			If(guid.Time(d)).Should().Equal(guid.Time(d))
	}
}

func TestG(t *testing.T) {
	c := guid.NewLClock()
	a := guid.G(c)
	b := guid.G(c)

	it.Ok(t).
		If(guid.Eq(a, b)).ShouldNot().Equal(true).
		If(guid.Lt(a, b)).Should().Equal(true).
		If(guid.Node(a)).Should().Equal(guid.Node(b)).
		If(guid.Seq(b) - guid.Seq(a)).Should().Equal(uint64(1))
}

func TestSpecG(t *testing.T) {
	spec := map[uint64]int64{
		1 << 16: 0,
		1 << 17: 1 << 17,
		1 << 24: 1 << 24,
		1 << 32: 1 << 32,
		1 << 62: 1 << 62,
	}

	// Note: if drift < 30 sec than node id is fits low bits only
	for _, d := range drifts[1:] {
		for tc, expect := range spec {
			c := guid.NewLClock(
				guid.NodeID(0xffffffff),
				guid.Clock(func() uint64 { return tc }),
			)
			a := guid.G(c, d)
			b := guid.G(c, d)

			it.Ok(t).
				If(guid.Eq(a, b)).ShouldNot().Equal(true).
				If(guid.Lt(a, b)).Should().Equal(true).
				If(guid.Seq(b) - guid.Seq(a)).Should().Equal(uint64(1)).
				If(guid.Time(a) == guid.Time(b)).Should().Equal(true).
				If(guid.Time(a)).Should().Equal(uint64(expect))
		}
	}
}

func TestDiffG(t *testing.T) {
	for i, drift := range drifts[1:] {
		c := guid.NewLClock(
			guid.NodeID(0xffffffff),
			guid.Clock(func() uint64 { return 1 << 17 }),
		)

		a := guid.G(c, drift)
		b := guid.G(c, drift)
		d := guid.Diff(b, a)
		bytes := guid.Bytes(d)

		it.Ok(t).
			If(guid.Seq(d)).Should().Equal(uint64(1)).
			If(guid.Time(d)).Should().Equal(uint64(0)).
			If(guid.Node(d)).Should().Equal(uint64(0xffffffff)).
			If(bytes[0]).Should().Equal(byte((i + 1) << 5)).
			If(bytes[11]).Should().Equal(byte(1))
	}
}

func TestDiffGZ(t *testing.T) {
	for _, drift := range drifts[1:] {
		c := guid.NewLClock(
			guid.NodeID(0xffffffff),
			guid.Clock(func() uint64 { return 1 << 17 }),
		)

		z := guid.ToG(c, guid.Z(c, drift))
		a := guid.G(c, drift)
		d := guid.Diff(a, z)

		it.Ok(t).
			If(guid.Eq(a, d)).Should().Eq(true).
			If(guid.Seq(d)).Should().Equal(guid.Seq(a)).
			If(guid.Time(d)).Should().Equal(guid.Time(a)).
			If(guid.Node(d)).Should().Equal(guid.Node(a))
	}
}

func TestLtoG(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewLClock(
			guid.NodeID(0xffffffff),
			guid.ClockUnix(),
		)

		a := guid.L(c, drift)
		b := guid.ToG(c, a)

		it.Ok(t).
			If(guid.Time(b)).Should().Equal(guid.Time(a)).
			If(guid.Seq(b)).Should().Equal(guid.Seq(a)).
			If(guid.Node(b)).Should().Equal(uint64(0xffffffff))
	}
}

/*
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

var last *guid.G

func BenchmarkL(b *testing.B) {
	var val guid.G
	for i := 0; i < b.N; i++ {
		val = guid.Seq.G()
	}
	last = &val
}
*/
