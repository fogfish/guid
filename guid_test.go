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

func TestGuidZ(t *testing.T) {
	c := guid.NewLClock()
	a := guid.G.Z(c)
	b := guid.G.Z(c)

	it.Ok(t).
		If(guid.G.Eq(a, b)).Should().Equal(true).
		If(guid.G.Lt(a, b)).ShouldNot().Equal(true).
		If(guid.G.Seq(a)).Should().Equal(uint64(0)).
		If(guid.G.Time(a)).Should().Equal(uint64(0)).
		If(guid.G.EpochT(a)).Should().Equal(time.Unix(0, 0))
}

func TestGuid(t *testing.T) {
	c := guid.NewLClock()
	a := guid.G.K(c)
	b := guid.G.K(c)

	it.Ok(t).
		If(guid.G.Eq(a, b)).ShouldNot().Equal(true).
		If(guid.G.Lt(a, b)).Should().Equal(true).
		If(guid.G.Node(a)).Should().Equal(guid.G.Node(b)).
		If(guid.G.Seq(b) - guid.G.Seq(a)).Should().Equal(uint64(1))
}

func TestGuidSpec(t *testing.T) {
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
				guid.ConfNodeID(0xffffffff),
				guid.ConfClock(func() uint64 { return tc }),
			)
			a := guid.G.K(c, d)
			b := guid.G.K(c, d)

			it.Ok(t).
				If(guid.G.Eq(a, b)).ShouldNot().Equal(true).
				If(guid.G.Lt(a, b)).Should().Equal(true).
				If(guid.G.Seq(b) - guid.G.Seq(a)).Should().Equal(uint64(1)).
				If(guid.G.Time(a) == guid.G.Time(b)).Should().Equal(true).
				If(guid.G.Time(a)).Should().Equal(uint64(expect))
		}
	}
}

func TestGuidDiff(t *testing.T) {
	for i, drift := range drifts[1:] {
		c := guid.NewLClock(
			guid.ConfNodeID(0xffffffff),
			guid.ConfClock(func() uint64 { return 1 << 17 }),
		)

		a := guid.G.K(c, drift)
		b := guid.G.K(c, drift)
		d := guid.G.Diff(b, a)
		bytes := guid.G.Bytes(d)

		it.Ok(t).
			If(guid.G.Seq(d)).Should().Equal(uint64(1)).
			If(guid.G.Time(d)).Should().Equal(uint64(0)).
			If(guid.G.Node(d)).Should().Equal(uint64(0xffffffff)).
			If(bytes[0]).Should().Equal(byte((i + 1) << 5)).
			If(bytes[11]).Should().Equal(byte(1))
	}
}

func TestGuidDiffZ(t *testing.T) {
	for _, drift := range drifts[1:] {
		c := guid.NewLClock(
			guid.ConfNodeID(0xffffffff),
			guid.ConfClock(func() uint64 { return 1 << 17 }),
		)

		z := guid.G.Z(c, drift)
		a := guid.G.K(c, drift)
		d := guid.G.Diff(a, z)

		it.Ok(t).
			If(guid.G.Eq(a, d)).Should().Eq(true).
			If(guid.G.Seq(d)).Should().Equal(guid.G.Seq(a)).
			If(guid.G.Time(d)).Should().Equal(guid.G.Time(a)).
			If(guid.G.Node(d)).Should().Equal(guid.G.Node(a))
	}
}

func TestGuidFromL(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewLClock(
			guid.ConfNodeID(0xffffffff),
			guid.ConfClockUnix(),
		)

		a := guid.L.K(c, drift)
		b := guid.G.FromL(c, a)

		it.Ok(t).
			If(guid.G.Time(b)).Should().Equal(guid.L.Time(a)).
			If(guid.G.Seq(b)).Should().Equal(guid.L.Seq(a)).
			If(guid.G.Node(b)).Should().Equal(uint64(0xffffffff))
	}
}

func TestGuidCodec(t *testing.T) {
	for i := 0; i <= 31; i++ {
		c := guid.NewLClock(
			guid.ConfNodeID(1<<i),
			guid.ConfClockUnix(),
		)

		a := guid.G.K(c)
		b := guid.G.FromBytes(guid.G.Bytes(a))
		d := guid.G.FromString(guid.G.String(a))

		it.Ok(t).
			If(guid.G.Eq(a, b)).Should().Equal(true).
			If(guid.G.Eq(a, d)).Should().Equal(true)
	}
}

func TestGuidFromT(t *testing.T) {
	for _, drift := range drifts {
		n := time.Now().Round(10 * time.Millisecond)

		a := guid.L.FromT(n, drift)
		b := guid.G.FromL(guid.Clock, a)
		v := guid.G.EpochT(b).Round(10 * time.Millisecond)

		it.Ok(t).
			If(v).Should().Equal(n)
	}
}

func TestGuidEpochT(t *testing.T) {
	n := time.Now().Round(10 * time.Millisecond)
	c := guid.NewLClock(
		guid.ConfClock(func() uint64 { return uint64(n.UnixNano()) }),
	)

	a := guid.G.K(c)
	v := guid.G.EpochT(a).Round(10 * time.Millisecond)

	it.Ok(t).
		If(v).Should().Equal(n)
}

func TestGuidEpochI(t *testing.T) {
	n := time.Now().Round(10 * time.Millisecond)
	c := guid.NewLClock(
		guid.ConfClock(func() uint64 { return 0xffffffffffffffff - uint64(n.UnixNano()) }),
	)

	a := guid.G.K(c)
	v := guid.G.EpochI(a).Round(10 * time.Millisecond)

	it.Ok(t).
		If(v).Should().Equal(n)
}

func TestGuidLexSorting(t *testing.T) {
	c := guid.NewLClock(
		guid.ConfNodeID(0xffffffff),
		guid.ConfClockUnix(),
	)

	a := guid.G.K(c).String()
	b := guid.G.K(c).String()

	it.Ok(t).
		If(a).ShouldNot().Equal(b).
		If(a).Should().Be().Less(b)
}

func TestGuidJSONCodec(t *testing.T) {
	type MyStruct struct {
		ID guid.GID `json:"id"`
	}

	c := guid.NewLClock(
		guid.ConfNodeID(0xffffffff),
		guid.ConfClockUnix(),
	)
	val := MyStruct{guid.G.K(c)}
	b, _ := json.Marshal(val)

	var x MyStruct
	json.Unmarshal(b, &x)

	it.Ok(t).
		If(guid.G.Eq(val.ID, x.ID)).Should().Equal(true)
}

// var last *guid.K

// func BenchmarkL(b *testing.B) {
// 	var val guid.K
// 	for i := 0; i < b.N; i++ {
// 		val = guid.G(guid.Clock)
// 	}
// 	last = &val
// }
