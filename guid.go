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

// Layout of k-ordered values.
//
// The library defines three types, each stored in exactly the number of bits
// its schema occupies, so that none pays for the others:
//
//	X  128-bit globally unique value ⟨𝒅,𝒕,𝒍,𝒔⟩, an RFC 9562 UUID, see extended.go
//	G   96-bit globally unique value ⟨𝒅,𝒕,𝒍,𝒔⟩, see global.go
//	L   64-bit locally unique value  ⟨𝒅,𝒕,𝒔⟩,   see local.go
//
// The three agree on every fraction but ⟨𝒍⟩: the same ⟨𝒅⟩ ladder, the same
// 47-bit truncated clock ⟨𝒕⟩ of 131 µs ticks, the same 14-bit sequence ⟨𝒔⟩ and
// the same field order, so the analysis of doc/proof.md is stated once and
// holds for all of them. They differ in how much room is left for the node
// identity — none for L, 32 bits for G, 58 bits for X.
const (
	// number of bits reserved for the ⟨𝒅⟩ drift code
	bitsDrift = 3
	// number of bits occupied by the ⟨𝒔⟩ sequence
	bitsSeq = 14
	// ⟨𝒕⟩ is truncated by this many bits, one tick is 2¹⁷ ns ≈ 131 µs
	bitsSeqDrift = bitsSeq + bitsDrift
	// number of bits occupied by the truncated clock ⟨𝒙⟩, which ⟨𝒅⟩ splits
	// into the epoch ⟨𝑬⟩ of 47 − 𝑫 bits and the low clock bits ⟨𝒙ₗ⟩ of 𝑫
	bitsTime = 47
	// number of bits occupied by the ⟨𝒍⟩ location of G
	bitsNode = 32
	// number of bits occupied by the ⟨𝒍⟩ location of X, which spends the room
	// the wider value leaves over on the node rather than on the clock
	bitsNodeX = 58
	// number of bits RFC 9562 reserves for the version field of a UUID
	bitsVer = 4
	// number of bits RFC 9562 reserves for the variant field of a UUID
	bitsVar = 2
	// number of payload bits of X — its 128, less the 6 that RFC 9562 reserves
	// for the version and the variant, see extended.go
	bitsPayloadX = SizeX*8 - bitsVer - bitsVar
	// bit position of the ⟨𝒅⟩ drift code within G, its most significant field
	posDrift = SizeG*8 - bitsDrift
	// mask of the ⟨𝒔⟩ sequence
	maskSeq = 1<<bitsSeq - 1
	// mask of the ⟨𝒅⟩ drift code
	maskDrift = 1<<bitsDrift - 1
	// mask of the ⟨𝒍⟩ location of G
	maskNode = 1<<bitsNode - 1
	// mask of the ⟨𝒍⟩ location of X. A clock carries its node identity at this
	// width and G truncates it to maskNode when it stamps a value with it.
	maskNodeX = 1<<bitsNodeX - 1
)

const (
	// SizeX is the size of X in bytes, both in memory and on the wire
	SizeX = 16
	// SizeG is the size of G in bytes, both in memory and on the wire
	SizeG = 12
	// SizeL is the size of L in bytes on the wire
	SizeL = 8
	// SizeString is the length of the lexicographically sortable string
	// encoding produced by String, for both G and L
	SizeString = 16
	// SizeStringX is the length of the canonical UUID string produced by
	// X.String, e.g. "06377f2a-0cb8-8000-8000-000000000001"
	SizeStringX = 36
)
