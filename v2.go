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

import (
	"fmt"
	"time"
)

// Reading a v2 G value with v3.
//
// v2's ⟨d⟩ code and v3's mean different 𝑫: v2 computed 𝑫 = 18 + code, v3
// reads 𝑫 from driftLadder[code] = {3,7,11,14,17,21,23,25}. Both schemas
// place ⟨𝑬⟩, ⟨𝒍⟩, ⟨𝒙ₗ⟩ and ⟨𝒔⟩ at the positions 𝑫 gives them — v2's own
// diagram in common.go is v3's Proposition 1 — so a value is decoded
// correctly by either version only where the two tables agree on 𝑫 for
// the stored code, which happens at exactly three of the eight codes:
//
//	code   v2 𝑫   v3 𝑫   agree
//	  0     18      3     no
//	  1     19      7     no
//	  2     20     11     no
//	  3     21     14     no   <- v2's default
//	  4     22     17     no
//	  5     23     21     no
//	  6     24     23     no
//	  7     25     25    yes
//
// So README's "the bit layout is unchanged" holds for L, which never splits
// a field by 𝑫, and holds for G only at code 7. Elsewhere v3's Node and Time
// read the wrong bit ranges — silently, since every code is a valid index
// into driftLadder and every extraction is in range. v2's own default (code
// 3) is among the seven that are wrong.
//
// FromV2 decodes with v2's table, so it recovers the values v2 meant, then
// re-encodes with v3's, choosing the narrowest v3 rung whose window is at
// least as wide as v2's — exact at codes 3, 5 and 7, where v2's window exists
// on v3's ladder, and rounded up elsewhere. Migrated values are therefore
// never less tolerant of clock skew than the v2 keyspace was.
//
// This is migration tooling for a compatibility break, not a permanent part
// of the API — hence the free function rather than the method-per-encoding
// shape the rest of the package uses, and hence the bool rather than two
// named entry points. Delete it once v2 keyspaces are gone.
const driftZv2 = 18

// FromV2 decodes a G produced by v2 of this library (module path
// github.com/fogfish/guid, before the /v3 split): val is a v2 String
// encoding if isBase62 is false, a v2 Base62 encoding if true. See the
// package comment above for why this needs to exist at all — do not decode a
// v2 value with G's own FromString or FromBase62, both silently misread
// ⟨𝒍⟩ and ⟨𝒕⟩ for seven of v2's eight drift settings.
//
// Use it once, to rewrite a v2 keyspace; the result is an ordinary v3 G and
// needs no further special handling.
func FromV2(isBase62 bool, val string) (G, error) {
	var raw G

	if isBase62 {
		b, err := decode62([]byte(val))
		if err != nil {
			return G{}, err
		}
		// base62 is a positional numeral system, it does not carry leading zeros
		if len(b) > SizeG {
			return G{}, fmt.Errorf("malformed k-order number: %v", val)
		}
		copy(raw[SizeG-len(b):], b)
	} else {
		if len(val) != SizeString {
			return G{}, fmt.Errorf("malformed k-order number: %v", val)
		}
		raw.Fold(6, decode64(val))
	}

	hi, lo := raw.words()

	d := hi >> (bitsNode - bitsDrift)
	D := driftZv2 + d

	node := extract(hi, lo, bitsSeq+D, bitsNode)
	e := extract(hi, lo, bitsSeq+D+bitsNode, bitsTime-D)
	x := extract(hi, lo, bitsSeq, D)
	t := (e<<D | x) << bitsSeqDrift
	seq := lo & maskSeq

	window := time.Duration(1) << (bitsSeqDrift + D)
	return makeG(node, DriftOf(window), t, seq), nil
}
