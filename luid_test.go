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

func TestLuidZ(t *testing.T) {
	c := guid.NewLClock()
	a := guid.L.Z(c)
	b := guid.L.Z(c)

	it.Ok(t).
		If(guid.L.Eq(a, b)).Should().Equal(true).
		If(guid.L.Lt(a, b)).ShouldNot().Equal(true).
		If(guid.L.Seq(a)).Should().Equal(uint64(0)).
		If(guid.L.Time(a)).Should().Equal(uint64(0)).
		If(guid.L.EpochT(a)).Should().Equal(time.Unix(0, 0))
}

func TestLuid(t *testing.T) {
	c := guid.NewLClock()
	a := guid.L.K(c)
	b := guid.L.K(c)

	it.Ok(t).
		If(guid.L.Eq(a, b)).ShouldNot().Equal(true).
		If(guid.L.Lt(a, b)).Should().Equal(true).
		If(guid.L.Lt(b, a)).Should().Equal(false).
		If(guid.L.Seq(b) - guid.L.Seq(a)).Should().Equal(uint64(1))
}

func TestLuidSpec(t *testing.T) {
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
				guid.ConfNodeID(0xffffffff),
				guid.ConfClock(func() uint64 { return tc }),
			)
			a := guid.L.K(c, d)
			b := guid.L.K(c, d)

			it.Ok(t).
				If(guid.L.Eq(a, b)).ShouldNot().Equal(true).
				If(guid.L.Lt(a, b)).Should().Equal(true).
				If(guid.L.Seq(b) - guid.L.Seq(a)).Should().Equal(uint64(1)).
				If(guid.L.Time(a) == guid.L.Time(b)).Should().Equal(true).
				If(guid.L.Time(a)).Should().Equal(uint64(expect))
		}
	}
}

func TestLuidDiff(t *testing.T) {
	for i, drift := range drifts {
		c := guid.NewLClock(
			guid.ConfNodeID(0xffffffff),
			guid.ConfClock(func() uint64 { return 1 << 17 }),
		)

		a := guid.L.K(c, drift)
		b := guid.L.K(c, drift)
		d := guid.L.Diff(b, a)

		it.Ok(t).
			If(guid.L.Seq(d)).Should().Equal(uint64(1)).
			If(guid.L.Time(d)).Should().Equal(uint64(0)).
			If(guid.L.Bytes(d)).Should().Equal([]byte{byte(i << 5), 0, 0, 0, 0, 0, 0, 1})
	}
}

func TestLuidDiffZ(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewLClock(
			guid.ConfNodeID(0xffffffff),
			guid.ConfClock(func() uint64 { return 1 << 17 }),
		)

		z := guid.L.Z(c, drift)
		a := guid.L.K(c, drift)
		d := guid.L.Diff(a, z)

		it.Ok(t).
			If(guid.L.Eq(d, a)).Should().Equal(true).
			If(guid.L.Seq(d)).Should().Equal(guid.L.Seq(a)).
			If(guid.L.Time(d)).Should().Equal(guid.L.Time(d))
	}
}

func TestLuidFromG(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewLClock(
			guid.ConfNodeID(0xffffffff),
			guid.ConfClockUnix(),
		)

		a := guid.G.K(c, drift)
		b := guid.L.FromG(a)

		it.Ok(t).
			If(guid.L.Time(b)).Should().Equal(guid.G.Time(a)).
			If(guid.L.Seq(b)).Should().Equal(guid.G.Seq(a))
	}
}

func TestLuidCodec(t *testing.T) {
	for i := 0; i <= 31; i++ {
		c := guid.NewLClock(
			guid.ConfNodeID(1<<i),
			guid.ConfClockUnix(),
		)

		a := guid.L.K(c)
		b := guid.L.FromBytes(guid.L.Bytes(a))
		d := guid.L.FromString(guid.L.String(a))

		it.Ok(t).
			If(guid.L.Eq(a, b)).Should().Equal(true).
			If(guid.L.Eq(a, d)).Should().Equal(true)
	}
}

func TestLuidFromT(t *testing.T) {
	for _, drift := range drifts {
		n := time.Now().Round(10 * time.Millisecond)

		a := guid.L.FromT(n, drift)
		v := guid.L.EpochT(a).Round(10 * time.Millisecond)

		it.Ok(t).
			If(v).Should().Equal(n)
	}
}

func TestLuidEpochT(t *testing.T) {
	n := time.Now().Round(10 * time.Millisecond)
	c := guid.NewLClock(
		guid.ConfClock(func() uint64 { return uint64(n.UnixNano()) }),
	)

	a := guid.L.K(c)
	v := guid.L.EpochT(a).Round(10 * time.Millisecond)

	it.Ok(t).
		If(v).Should().Equal(n)
}

func TestLuidEpochI(t *testing.T) {
	n := time.Now().Round(10 * time.Millisecond)
	c := guid.NewLClock(
		guid.ConfClock(func() uint64 { return 0xffffffffffffffff - uint64(n.UnixNano()) }),
	)

	a := guid.L.K(c)
	v := guid.L.EpochI(a).Round(10 * time.Millisecond)

	it.Ok(t).
		If(v).Should().Equal(n)
}

func TestLuidLexSorting(t *testing.T) {
	c := guid.NewLClock(
		guid.ConfNodeID(0xffffffff),
		guid.ConfClockUnix(),
	)

	a := guid.L.K(c).String()
	b := guid.L.K(c).String()

	it.Ok(t).
		If(a).ShouldNot().Equal(b).
		If(a).Should().Be().Less(b)
}

func TestLuidJSONCodec(t *testing.T) {
	type MyStruct struct {
		ID guid.LID `json:"id"`
	}

	c := guid.NewLClock(
		guid.ConfNodeID(0xffffffff),
		guid.ConfClockUnix(),
	)
	val := MyStruct{guid.L.K(c)}
	b, _ := json.Marshal(val)

	var x MyStruct
	json.Unmarshal(b, &x)

	it.Ok(t).
		If(guid.L.Eq(val.ID, x.ID)).Should().Equal(true)
}
