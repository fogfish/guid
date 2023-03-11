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

import "time"

// zero point for drift
const driftZ = 18

// driftBits converts a time drift into number of bits to shift the location
// fraction. E.g. if application allows 2 min time drift in the system than last
// 20 bits of timestamp becomes less significant than location.
//
// The default drift is approximately 5 min, the drift value is encoded as
// 3 bits, which gives 8 possible values
func driftInBits(drift []time.Duration) uint64 {
	switch {
	case len(drift) == 0:
		return driftZ + 3
	case drift[0] <= 34*time.Second:
		return driftZ
	case drift[0] <= 68*time.Second:
		return driftZ + 1
	case drift[0] <= 137*time.Second:
		return driftZ + 2
	case drift[0] <= 274*time.Second:
		return driftZ + 3
	case drift[0] <= 549*time.Second:
		return driftZ + 4
	case drift[0] <= 1099*time.Second:
		return driftZ + 5
	case drift[0] <= 2199*time.Second:
		return driftZ + 6
	default:
		return driftZ + 7
	}
}

// splits ⟨𝒕⟩ faction (timestamp) to hi and lo bits of K order value
func splitT(t uint64, drift uint64) (uint64, uint64) {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	// 14 bits of time is exchange for seq
	//  3 bits is reserved for drift
	//    initial timestamp is reduced by 17 bits ~ 10⁶ nanoseconds
	x := t >> (14 + 3)
	a := 64 - 14 - drift
	b := 32 - a

	lo := (x << (a + 14)) >> a
	hi := (x >> drift) << b
	dd := (drift - driftZ) << 29

	return hi | dd, lo
}

// split ⟨𝒍⟩ faction (location) to hi and lo bits of K order value
func splitNode(node, drift uint64) (uint64, uint64) {
	//
	//   3    47 - drift             32bit      drift   14
	//  |-|-------------------|--------!-------|-----|-------|
	//  ^                         b    ^   a                 ^
	// 96                             64                     0
	//
	a := 64 - 14 - drift
	b := 32 - a

	lo := node << (drift + 14)
	hi := node >> (32 - b)

	return hi, lo
}

func split(hi, lo, size, n uint64) (bytes []byte) {
	hilo := uint64(64) // hi | lo division at
	bytes = make([]byte, size/n)

	mask := uint64(1<<n) - 1
	i := 0

	for a := size; a >= n; a -= n {
		b := a - n
		switch {
		case a >= hilo && b >= hilo:
			value := byte(hi >> (b - hilo) & mask)
			bytes[i] = value
		case a <= hilo && b <= hilo:
			value := byte(lo >> b & mask)
			bytes[i] = value
		case a > hilo && b < hilo:
			suffix := uint64(1<<(a-hilo)) - 1
			hi := byte(hi & suffix)
			lo := byte(lo >> b)
			bytes[i] = hi<<(hilo-b) | lo
		}
		i++
	}

	return
}

func fold(size, n uint64, bytes []byte) (hi, lo uint64) {
	hilo := uint64(64)

	mask := uint64(1<<n) - 1
	i := 0

	for a := size; a >= n; a -= n {
		b := a - n
		switch {
		case a >= hilo && b >= hilo:
			hi |= (uint64(bytes[i]) & mask) << (b - hilo)
		case a <= hilo && b <= hilo:
			lo |= (uint64(bytes[i]) & mask) << b
		case a > hilo && b < hilo:
			hi |= (uint64(bytes[i]) & mask) >> (hilo - b)
			lo |= (uint64(bytes[i]) & mask) << b
		}
		i++
	}
	return
}
