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

	"github.com/fogfish/guid/v2"
	"github.com/fogfish/it/v2"
)

var drifts []time.Duration = []time.Duration{
	// 30 * time.Second,
	60 * time.Second,
	130 * time.Second,
	270 * time.Second,
	540 * time.Second,
	1000 * time.Second,
	2100 * time.Second,
	3600 * time.Second,
}

func TestZ(t *testing.T) {
	c := guid.NewClock()
	a := guid.Z(c)
	b := guid.Z(c)

	it.Then(t).Should(
		it.Equiv(a, b),
		it.Equal(guid.Seq(a), 0),
		it.Equal(guid.Time(a), 0),
	).ShouldNot(
		it.True(guid.Before(a, b)),
		it.True(guid.After(a, b)),
	)
}

func TestG(t *testing.T) {
	c := guid.NewClock()
	a := guid.G(c)
	b := guid.G(c)

	it.Then(t).ShouldNot(
		it.Equiv(a, b),
		it.True(guid.After(a, b)),
		it.True(guid.Before(b, a)),
	).Should(
		it.True(guid.Before(a, b)),
		it.True(guid.After(b, a)),
		it.Equal(guid.Node(a), guid.Node(b)),
		it.Equal(guid.Seq(b)-guid.Seq(a), 1),
	)
}

func TestL(t *testing.T) {
	c := guid.NewClock()
	a := guid.L(c)
	b := guid.L(c)

	it.Then(t).ShouldNot(
		it.Equiv(a, b),
		it.True(guid.After(a, b)),
		it.True(guid.Before(b, a)),
	).Should(
		it.True(guid.Before(a, b)),
		it.True(guid.After(b, a)),
		it.Equal(guid.Node(a), guid.Node(b)),
		it.Equal(guid.Node(a), 0),
		it.Equal(guid.Seq(b)-guid.Seq(a), 1),
	)
}

func TestAfter(t *testing.T) {
	for a, b := range map[string]string{
		"NiiTRfl2BaVI1B.0": "NiiTTfl2BaVBHo8R",
		"NiiTRfl2BaVI1B.1": "NiiTTfl2BaV71R8Q",
	} {
		av, _ := guid.FromStringG(a)
		bv, _ := guid.FromStringG(b)

		it.Then(t).Should(
			it.True(guid.After(bv, av)),
			it.True(guid.Before(av, bv)),
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

	// Note: if drift < 30 sec than node id is fits low bits only
	for _, d := range drifts[1:] {
		for tc, expect := range spec {
			c := guid.NewClock(
				guid.WithNodeID(0xffffffff),
				guid.WithClock(func() uint64 { return tc }),
			)
			a := guid.G(c, d)
			b := guid.G(c, d)

			it.Then(t).ShouldNot(
				it.Equiv(a, b),
			).Should(
				it.True(guid.Before(a, b)),
				it.True(guid.After(b, a)),
				it.Equal(guid.Seq(b)-guid.Seq(a), 1),
				it.Equal(guid.Time(a), guid.Time(b)),
				it.Equal(guid.Time(a), uint64(expect)),
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
				guid.WithNodeID(0xffffffff),
				guid.WithClock(func() uint64 { return tc }),
			)
			a := guid.L(c, d)
			b := guid.L(c, d)

			it.Then(t).ShouldNot(
				it.Equal(a, b),
			).Should(
				it.True(guid.Before(a, b)),
				it.True(guid.After(b, a)),
				it.Equal(guid.Seq(b)-guid.Seq(a), 1),
				it.Equal(guid.Time(a), guid.Time(b)),
				it.Equal(guid.Time(a), uint64(expect)),
			)
		}
	}
}

func TestDiffG(t *testing.T) {
	for i, drift := range drifts {
		c := guid.NewClock(
			guid.WithNodeID(0xffffffff),
			guid.WithClock(func() uint64 { return 1 << 17 }),
		)

		a := guid.G(c, drift)
		b := guid.G(c, drift)
		d := guid.Diff(b, a)
		bytes := guid.Bytes(d)

		it.Then(t).Should(
			it.Equal(guid.Seq(d), 1),
			it.Equal(guid.Time(d), 0),
			it.Equal(guid.Node(d), 0xffffffff),
			it.Equal(bytes[0], byte((i+1)<<5)),
			it.Equal(bytes[11], 1),
		)
	}
}

func TestDiffL(t *testing.T) {
	for i, drift := range drifts {
		c := guid.NewClock(
			guid.WithNodeID(0xffffffff),
			guid.WithClock(func() uint64 { return 1 << 17 }),
		)

		a := guid.L(c, drift)
		b := guid.L(c, drift)
		d := guid.Diff(b, a)

		it.Then(t).Should(
			it.Equal(guid.Seq(d), 1),
			it.Equal(guid.Time(d), 0),
			it.Equiv(guid.Bytes(d), []byte{byte((i + 1) << 5), 0, 0, 0, 0, 0, 0, 1}),
		)
	}
}

func TestDiffGZ(t *testing.T) {
	for _, drift := range drifts[1:] {
		c := guid.NewClock(
			guid.WithNodeID(0xffffffff),
			guid.WithClock(func() uint64 { return 1 << 17 }),
		)

		z := guid.Z(c, drift)
		a := guid.G(c, drift)
		d := guid.Diff(a, z)

		it.Then(t).Should(
			it.True(guid.Equal(a, d)),
			it.Equal(guid.Seq(d), guid.Seq(a)),
			it.Equal(guid.Time(d), guid.Time(a)),
			it.Equal(guid.Node(d), guid.Node(a)),
		)
	}
}

func TestDiffLZ(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(
			guid.WithNodeID(0xffffffff),
			guid.WithClock(func() uint64 { return 1 << 17 }),
		)

		z := guid.ToL(guid.Z(c, drift))
		a := guid.L(c, drift)
		d := guid.Diff(a, z)

		it.Then(t).Should(
			it.True(guid.Equal(a, d)),
			it.Equal(guid.Seq(d), guid.Seq(a)),
			it.Equal(guid.Time(d), guid.Time(a)),
		)
	}
}

func TestFromL(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(
			guid.WithNodeID(0xffffffff),
			guid.WithClockUnix(),
		)

		for _, a := range []guid.K{
			guid.L(c, drift),
			guid.G(c, drift),
		} {
			b := guid.FromL(c, a)

			it.Then(t).Should(
				it.Equal(guid.Time(b), guid.Time(a)),
				it.Equal(guid.Seq(b), guid.Seq(a)),
				it.Equal(guid.Node(b), 0xffffffff),
			)
		}
	}
}

func TestToL(t *testing.T) {
	for _, drift := range drifts {
		c := guid.NewClock(
			guid.WithNodeID(0xffffffff),
			guid.WithClockUnix(),
		)

		for _, a := range []guid.K{
			guid.G(c, drift),
			guid.L(c, drift),
		} {
			b := guid.ToL(a)

			it.Then(t).Should(
				it.Equal(guid.Time(b), guid.Time(a)),
				it.Equal(guid.Seq(b), guid.Seq(a)),
			)
		}
	}
}

func TestCodecG(t *testing.T) {
	for i := 0; i <= 31; i++ {
		c := guid.NewClock(
			guid.WithNodeID(1<<i),
			guid.WithClockUnix(),
		)

		a := guid.G(c)

		b, err := guid.FromBytes(guid.Bytes(a))
		it.Then(t).Should(
			it.Nil(err),
			it.Equiv(b, a),
		)

		d, err := guid.FromStringG(guid.String(a))
		it.Then(t).Should(
			it.Nil(err),
			it.Equiv(d, a),
		)

		x, err := guid.FromBase62(guid.Base62(a))
		it.Then(t).Should(
			it.Nil(err),
			it.Equiv(x, a),
		)
	}

	t.Run("Base62.Error", func(t *testing.T) {
		_, err := guid.FromBase62("......")
		it.Then(t).ShouldNot(it.Nil(err))
	})
}

func TestCodecL(t *testing.T) {
	for i := 0; i <= 31; i++ {
		c := guid.NewClock(
			guid.WithNodeID(1<<i),
			guid.WithClockUnix(),
		)

		a := guid.L(c)

		b, err := guid.FromBytes(guid.Bytes(a))
		it.Then(t).Should(
			it.Nil(err),
			it.Equiv(b, a),
		)

		d, err := guid.FromStringL(guid.String(a))
		it.Then(t).Should(
			it.Nil(err),
			it.Equiv(d, a),
		)

		x, err := guid.FromBase62(guid.Base62(a))
		it.Then(t).Should(
			it.Nil(err),
			it.Equiv(x, a),
		)

		_, err = guid.FromStringL("xxxxxx")
		it.Then(t).ShouldNot(it.Nil(err))

		_, err = guid.FromBytes([]byte("xxxxxx"))
		it.Then(t).ShouldNot(it.Nil(err))
	}
}

func TestFromT(t *testing.T) {
	for _, drift := range drifts {
		n := time.Now().Round(10 * time.Millisecond)

		a := guid.FromT(n, drift)
		b := guid.FromL(guid.Clock, a)
		v := guid.EpochT(b).Round(10 * time.Millisecond)

		it.Then(t).Should(
			it.Equal(v, n),
		)
	}
}

func TestEpochT(t *testing.T) {
	n := time.Now().Round(10 * time.Millisecond)
	c := guid.NewClock(
		guid.WithClock(func() uint64 { return uint64(n.UnixNano()) }),
	)

	a := guid.G(c)
	v := guid.EpochT(a).Round(10 * time.Millisecond)

	b := guid.L(c)
	w := guid.EpochT(b).Round(10 * time.Millisecond)

	it.Then(t).Should(
		it.Equal(v, n),
		it.Equal(w, n),
	)
}

func TestEpochI(t *testing.T) {
	n := time.Now().Round(10 * time.Millisecond)
	c := guid.NewClock(
		guid.WithClock(func() uint64 { return 0xffffffffffffffff - uint64(n.UnixNano()) }),
	)

	a := guid.G(c)
	v := guid.EpochI(a).Round(10 * time.Millisecond)

	b := guid.L(c)
	w := guid.EpochI(b).Round(10 * time.Millisecond)

	it.Then(t).Should(
		it.Equal(v, n),
		it.Equal(w, n),
	)
}

func TestLexSorting(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xffffffff),
		guid.WithClockUnix(),
	)

	a := guid.G(c).String()
	b := guid.G(c).String()
	it.Then(t).ShouldNot(
		it.Equal(a, b),
	).Should(
		it.Less(a, b),
	)

	e := guid.L(c).String()
	f := guid.L(c).String()

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

	a := guid.Base62(guid.G(c))
	b := guid.Base62(guid.G(c))
	it.Then(t).ShouldNot(
		it.Equal(a, b),
	).Should(
		it.Less(a, b),
	)

	e := guid.Base62(guid.L(c))
	f := guid.Base62(guid.L(c))

	it.Then(t).ShouldNot(
		it.Equal(e, f),
	).Should(
		it.Less(e, f),
	)
}

func TestSplit(t *testing.T) {
	c := guid.NewClock(
		guid.WithNodeID(0xffffffff),
		guid.WithClockUnix(),
	)

	a := guid.G(c)
	b := guid.L(c)

	it.Then(t).Should(
		it.Equiv(guid.Bytes(a), guid.Split(8, a)),
		it.Equiv(guid.Bytes(b), guid.Split(8, b)),
	)
}

func TestJSONCodec(t *testing.T) {
	type MyStruct struct {
		G guid.K `json:"g"`
		L guid.K `json:"l"`
	}

	c := guid.NewClock(
		guid.WithNodeID(0xffffffff),
		guid.WithClockUnix(),
	)
	val := MyStruct{G: guid.G(c), L: guid.L(c)}
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
	type MyStruct struct {
		ID guid.K `json:"id"`
	}

	for _, tt := range []string{
		`{"id":100}`,
		`{"id":"*****"}`,
	} {
		var x MyStruct
		err := json.Unmarshal([]byte(tt), &x)
		it.Then(t).ShouldNot(
			it.Nil(err),
		)
	}
}

var (
	k guid.K
	s string
	d []byte
	t uint64
)

func BenchmarkGUID(b *testing.B) {
	b.Run("G", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			k = guid.G(guid.Clock)
		}
	})

	b.Run("L", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			k = guid.L(guid.Clock)
		}
	})

	b.Run("String", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s = guid.String(guid.G(guid.Clock))
		}
	})

	b.Run("Bytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			d = guid.Bytes(guid.G(guid.Clock))
		}
	})

	b.Run("Time", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			t = guid.Time(guid.G(guid.Clock))
		}
	})

	b.Run("Node", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			t = guid.Node(guid.G(guid.Clock))
		}
	})

}
