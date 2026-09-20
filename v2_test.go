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
	"testing"
	"time"

	"github.com/fogfish/guid/v3"
	"github.com/fogfish/it/v2"
)

// v2G packs a value exactly as v2 of this library did (module
// github.com/fogfish/guid, common.go/guid.go at tag v2.1.0): the same field
// order, but ⟨d⟩'s code means 𝑫 = 18 + code rather than a driftLadder lookup.
// It exists only so these tests can produce input FromV2 has to decode,
// independent of the v2 module (which does not build under v3's module path).
func v2G(node, drift, t, seq uint64) guid.G {
	x := t >> 17
	a := 64 - 14 - drift
	b := 32 - a

	// splitT
	tlo := (x << (a + 14)) >> a
	thi := (x >> drift) << b
	dd := (drift - 18) << 29

	// splitNode
	nlo := node << (drift + 14)
	nhi := node >> (32 - b)

	hi := uint32(thi | dd | nhi)
	lo := tlo | nlo | seq

	var uid guid.G
	if err := uid.FromBytes(foldBytes(hi, lo)); err != nil {
		panic(err)
	}
	return uid
}

func foldBytes(hi uint32, lo uint64) []byte {
	b := make([]byte, 12)
	b[0] = byte(hi >> 24)
	b[1] = byte(hi >> 16)
	b[2] = byte(hi >> 8)
	b[3] = byte(hi)
	for i := 0; i < 8; i++ {
		b[4+i] = byte(lo >> (56 - 8*i))
	}
	return b
}

func v2String(uid guid.G) string {
	return uid.String()
}

func v2Base62(uid guid.G) string {
	return uid.Base62()
}

// The seven codes v3 reads with a different 𝑫 than v2 meant, verified against
// the actual v2.1.0 packing formula. Node and Time must come back exactly as
// v2 allocated them — that is the whole claim of FromV2 — for every code, not
// only the one (7) that happens to survive naive decoding.
func TestFromV2RecoversEveryCode(t *testing.T) {
	const (
		node = uint64(0xAABBCCDD)
		tns  = uint64(1700000000_123456789)
		seq  = uint64(0x1234)
	)

	for code := uint64(0); code < 8; code++ {
		drift := 18 + code

		v2 := v2G(node, drift, tns, seq)
		s := v2String(v2)

		v3, err := guid.FromV2(false, s)

		it.Then(t).Should(
			it.Nil(err),
			it.Equal(v3.Node(), node),
			it.Equal(v3.Time(), tns>>17<<17),
			it.Equal(v3.Seq(), seq),
		)
	}
}

// The concrete failure FromV2 exists to avoid: decoding a v2 string with the
// ordinary FromString does not error — every 3-bit code is a valid index
// into v3's ladder — but Node and Time silently disagree with what v2
// allocated, at v2's own default drift (code 3).
func TestFromStringMisreadsV2Default(t *testing.T) {
	const (
		node = uint64(0xAABBCCDD)
		tns  = uint64(1700000000_123456789)
		seq  = uint64(0x1234)
	)

	v2 := v2G(node, 18+3, tns, seq) // v2's default: code 3, its 𝑫 = 21
	s := v2String(v2)

	var naive guid.G
	err := naive.FromString(s)

	it.Then(t).Should(
		it.Nil(err), // it decodes without error
		it.Equal(naive.Seq(), seq),
	)
	// but the location and time it reports are not what v2 allocated
	if naive.Node() == node {
		t.Fatal("expected FromString to misread ⟨l⟩ for a v2 value, it did not")
	}
}

// At codes 1, 3, 5 and 7 v2's window already exists on v3's ladder, so the
// migrated value's drift is exact, not merely wide enough.
func TestFromV2IsExactAtSharedRungs(t *testing.T) {
	cases := []struct {
		code  uint64
		drift guid.Drift
	}{
		{1, guid.Drift68s},   // v2 𝑫=19 (68.7s)
		{3, guid.Drift275s},  // v2 𝑫=21 (274.9s), v2's default
		{5, guid.Drift1099s}, // v2 𝑫=23 (1099s)
		{7, guid.Drift4398s}, // v2 𝑫=25 (4398s)
	}

	for _, c := range cases {
		v2 := v2G(0x1, 18+c.code, 0, 0)
		s := v2String(v2)

		v3, err := guid.FromV2(false, s)
		it.Then(t).Should(
			it.Nil(err),
			it.Equal(v3.Drift(), c.drift),
		)
	}
}

// At the four codes with no exact match, the migrated drift is the narrowest
// v3 rung that is at least as wide — never narrower than the v2 window, and
// never more than one rung, a factor of two, wider than it.
func TestFromV2NeverNarrowsTheWindow(t *testing.T) {
	cases := []struct {
		code  uint64
		drift guid.Drift
	}{
		{0, guid.Drift68s},   // v2 𝑫=18 (34.4s)  -> smallest v3 rung >= it: 68.7s
		{2, guid.Drift275s},  // v2 𝑫=20 (137.4s) -> 274.9s
		{4, guid.Drift1099s}, // v2 𝑫=22 (549.8s) -> 1099s
		{6, guid.Drift4398s}, // v2 𝑫=24 (2199s)  -> 4398s
	}

	for _, c := range cases {
		v2 := v2G(0x1, 18+c.code, 0, 0)
		s := v2String(v2)

		v3, err := guid.FromV2(false, s)
		it.Then(t).Should(
			it.Nil(err),
			it.Equal(v3.Drift(), c.drift),
			it.Less(int64(v2ExpectedWindow(c.code)), int64(v3.Drift().Window())+1),
		)
	}
}

func v2ExpectedWindow(code uint64) time.Duration {
	return time.Duration(1) << (17 + 18 + code)
}

func TestFromV2RejectsMalformed(t *testing.T) {
	_, err := guid.FromV2(false, "too-short")
	it.Then(t).ShouldNot(it.Nil(err))
}

// base62.go is byte-for-byte identical between v2 and v3 (verified against
// the actual v2.1.0 source), so the isBase62 path has to recover exactly what
// the String path does — same reinterpretation, a different wire encoding in
// front of it.
func TestFromV2Base62RecoversEveryCode(t *testing.T) {
	const (
		node = uint64(0xAABBCCDD)
		tns  = uint64(1700000000_123456789)
		seq  = uint64(0x1234)
	)

	for code := uint64(0); code < 8; code++ {
		drift := 18 + code

		v2 := v2G(node, drift, tns, seq)
		s := v2Base62(v2)

		v3, err := guid.FromV2(true, s)

		it.Then(t).Should(
			it.Nil(err),
			it.Equal(v3.Node(), node),
			it.Equal(v3.Time(), tns>>17<<17),
			it.Equal(v3.Seq(), seq),
		)
	}
}

// The two encodings of the same v2 value must reinterpret to the same v3
// value — they share the drift table and the field extraction, and differ
// only in how the 12 raw bytes were recovered.
func TestFromV2StringAndBase62Agree(t *testing.T) {
	v2 := v2G(0xAABBCCDD, 18+3, 1700000000_123456789, 0x1234)

	byString, err1 := guid.FromV2(false, v2String(v2))
	byBase62, err2 := guid.FromV2(true, v2Base62(v2))

	it.Then(t).Should(
		it.Nil(err1),
		it.Nil(err2),
		it.Equal(byString, byBase62),
	)
}

func TestFromV2Base62RejectsMalformed(t *testing.T) {
	_, err := guid.FromV2(true, "\x00corrupt")
	it.Then(t).ShouldNot(it.Nil(err))
}

// Base62 is a positional numeral system with no fixed width, so a decoded
// value wider than SizeG is malformed rather than merely large.
func TestFromV2Base62RejectsOversized(t *testing.T) {
	_, err := guid.FromV2(true, "zzzzzzzzzzzzzzzzzzzz")
	it.Then(t).ShouldNot(it.Nil(err))
}
