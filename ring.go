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

package guid

// Ring order — comparison relative to a point on a consistent hashing ring.
//
// Before compares ⟨𝒍⟩ with <, whose least element is 0. That is the member of
// a family of orders cut at the origin, and a ring has one such order per cut
// point rather than one order overall. For a key 𝒌 the successor list is the
// nodes clockwise from 𝒌, so the order the ring means is
//
//	𝒂 <𝒌 𝒃  ⟺  (𝒂 − 𝒌) mod 2ᴺ  <  (𝒃 − 𝒌) mod 2ᴺ
//
// which is a strict total order for every fixed 𝒌 — irreflexive, transitive,
// trichotomous. Two keys disagreeing about two nodes is not a contradiction,
// because <𝒌₁ and <𝒌₂ are different relations:
//
//	successors(𝒌₁) = [B C A]   is   B <𝒌₁ C <𝒌₁ A
//	successors(𝒌₂) = [C A B]   is   C <𝒌₂ A <𝒌₂ B
//
// Comparing with Before instead answers for the cut at 0, which is right only
// on the arc whose successor list does not wrap the origin — the misfire
// doc/vnode.md tabulates. Under <𝒌 the primary for 𝒌 is by construction the
// least element, at every key and for any placement of tokens.
//
// # What this costs
//
// Before is two word comparisons over the packed value, and the byte order of
// a stored value is that order (doc/proof.md §1.4), so an external index sorts
// correctly without decoding. Neither holds here. ⟨𝒍⟩ has to be read out of
// the middle of the value — at the default rung it straddles the hi/lo word
// boundary of a G — rotated, and compared apart from the fields around it, and
// no index can apply that. The rotation cannot be folded into storage either,
// since 𝒌 varies per query.
//
// This is why ring order is a separate comparator rather than the order of the
// type. It is meant for conflict resolution, over the few values that contend
// for one key, never for the index path: scanning does not need it, because
// contiguity of a node's run inside an epoch holds under Before already
// (doc/proof.md §4.2′) and rotation permutes the runs without splitting them.
//
// # Scope
//
// The comparator is the mechanism, not the policy. That the least element of
// <𝒌 wins a conflict is the application's rule to make, see doc/vnode.md.
//
// L has no ⟨𝒍⟩ and therefore no ring order.

// OrdRingG is the order on G cut at a ring position — the key whose successor
// list the order is to agree with. It is that position, so the conversion is
// the constructor:
//
//	ord := guid.OrdRingG(token >> 32)
//	slices.SortFunc(uids, ord.Compare)
//
// The position is taken at the width G gives ⟨𝒍⟩ and masked to it, the same
// truncation WithNodeID undergoes when a value is stamped, so a token wider
// than the field is to be shifted down here exactly as it is there. Ref
// reports the position actually compared against.
//
// The zero value is the order cut at the origin, which is Before.
type OrdRingG uint64

// OrdRingX is the order on X cut at a ring position, see OrdRingG. X gives ⟨𝒍⟩
// 58 bits rather than 32, so a 64-bit ring token is shifted by 6:
//
//	ord := guid.OrdRingX(token >> 6)
//	slices.SortFunc(uids, ord.Compare)
//
// The zero value is the order cut at the origin, which is Before.
type OrdRingX uint64

// Ref returns the ring position the order is cut at, at the width of ⟨𝒍⟩. A
// position wider than the field reads back truncated, because that is the part
// of it the comparison uses.
func (ord OrdRingG) Ref() uint64 { return uint64(ord) & maskNode }

// Ref returns the ring position the order is cut at, at the width of ⟨𝒍⟩. A
// position wider than the field reads back truncated, because that is the part
// of it the comparison uses.
func (ord OrdRingX) Ref() uint64 { return uint64(ord) & maskNodeX }

// Before reports whether a precedes b in the order cut at Ref.
func (ord OrdRingG) Before(a, b G) bool { return ord.Compare(a, b) < 0 }

// Before reports whether a precedes b in the order cut at Ref.
func (ord OrdRingX) Before(a, b X) bool { return ord.Compare(a, b) < 0 }

// After reports whether a follows b in the order cut at Ref.
func (ord OrdRingG) After(a, b G) bool { return ord.Compare(a, b) > 0 }

// After reports whether a follows b in the order cut at Ref.
func (ord OrdRingX) After(a, b X) bool { return ord.Compare(a, b) > 0 }

// Compare orders a and b relative to Ref, returning -1, 0 or +1. The signature
// is the one slices.SortFunc and slices.BinarySearchFunc take.
//
// The comparison is lexicographic on ⟨𝒅⟩, ⟨𝑬⟩, the rotated ⟨𝒍⟩ and ⟨𝒙ₗ⟩⟨𝒔⟩.
// Only ⟨𝒍⟩ is rotated: ⟨𝒅⟩ segregates the rungs and ⟨𝑬⟩ dominates everything
// below it exactly as it does under Before, so ring order permutes values
// within one epoch of one rung and nowhere else.
func (ord OrdRingG) Compare(a, b G) int {
	if da, db := a.Drift(), b.Drift(); da != db {
		return cmp(uint64(da), uint64(db))
	}

	ahi, alo := a.words()
	bhi, blo := b.words()
	d := a.Drift().Bits()

	if c := cmp(
		extract(ahi, alo, bitsSeq+d+bitsNode, bitsTime-d),
		extract(bhi, blo, bitsSeq+d+bitsNode, bitsTime-d),
	); c != 0 {
		return c
	}

	if c := cmp(
		(extract(ahi, alo, bitsSeq+d, bitsNode)-uint64(ord))&maskNode,
		(extract(bhi, blo, bitsSeq+d, bitsNode)-uint64(ord))&maskNode,
	); c != 0 {
		return c
	}

	return cmp(
		extract(ahi, alo, 0, bitsSeq+d),
		extract(bhi, blo, 0, bitsSeq+d),
	)
}

// Compare orders a and b relative to Ref, returning -1, 0 or +1. The signature
// is the one slices.SortFunc and slices.BinarySearchFunc take.
//
// The comparison is lexicographic on ⟨𝒅⟩, ⟨𝑬⟩, the rotated ⟨𝒍⟩ and ⟨𝒙ₗ⟩⟨𝒔⟩ of
// the payload, so the version and variant RFC 9562 fixes take no part in it —
// they are constants, and §1.2′ of doc/proof.md shows the splice changes no
// order.
func (ord OrdRingX) Compare(a, b X) int {
	if da, db := a.Drift(), b.Drift(); da != db {
		return cmp(uint64(da), uint64(db))
	}

	ahi, alo := a.payload()
	bhi, blo := b.payload()
	d := a.Drift().Bits()

	if c := cmp(
		extract(ahi, alo, bitsSeq+d+bitsNodeX, bitsTime-d),
		extract(bhi, blo, bitsSeq+d+bitsNodeX, bitsTime-d),
	); c != 0 {
		return c
	}

	if c := cmp(
		(extract(ahi, alo, bitsSeq+d, bitsNodeX)-uint64(ord))&maskNodeX,
		(extract(bhi, blo, bitsSeq+d, bitsNodeX)-uint64(ord))&maskNodeX,
	); c != 0 {
		return c
	}

	return cmp(
		extract(ahi, alo, 0, bitsSeq+d),
		extract(bhi, blo, 0, bitsSeq+d),
	)
}

func cmp(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return +1
	default:
		return 0
	}
}
