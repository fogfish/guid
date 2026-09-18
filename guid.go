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
// The library defines two types, each stored in exactly the number of bits its
// schema occupies, so that neither pays for the other:
//
//	G  96-bit globally unique value ⟨𝒅,𝒕,𝒍,𝒔⟩, see global.go
//	L  64-bit locally unique value  ⟨𝒅,𝒕,𝒔⟩,   see local.go
const (
	// number of bits reserved for the ⟨𝒅⟩ drift code
	bitsDrift = 3
	// number of bits occupied by the ⟨𝒔⟩ sequence
	bitsSeq = 14
	// ⟨𝒕⟩ is truncated by this many bits, one tick is 2¹⁷ ns ≈ 131 µs
	bitsSeqDrift = bitsSeq + bitsDrift
	// mask of the ⟨𝒔⟩ sequence
	maskSeq = 1<<bitsSeq - 1
)

const (
	// SizeG is the size of G in bytes, both in memory and on the wire
	SizeG = 12
	// SizeL is the size of L in bytes on the wire
	SizeL = 8
	// SizeString is the length of the lexicographically sortable string
	// encoding produced by String, for both G and L
	SizeString = 16
)
