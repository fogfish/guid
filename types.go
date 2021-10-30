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

/*

Chronos is a logical clock
*/
type Chronos interface {
	// Spatially unique identifier ⟨𝒍⟩ of ID allocator so called node location
	L() uint64
	// Monotonically increasing logical clock ⟨𝒕⟩
	T() uint64
}

/*

LClock is a default logical clock behavior
*/
type LClock struct {
	// Spatially unique identifier ⟨𝒍⟩
	location uint64
	// Monotonically increasing logical clock ⟨𝒕⟩ generator
	ticker func() uint64
}

// L returns spatially unique identifier ⟨𝒍⟩, so called node location
func (clock LClock) L() uint64 { return clock.location }

// T returns monotonically increasing logical clock ⟨𝒕⟩
func (clock LClock) T() uint64 { return clock.ticker() }

/*

Config option of default logical clock behavior.
Config options allows to define custom strategies to generate
⟨𝒍⟩ location or ⟨𝒕⟩ timestamp.
*/
type Config func(*LClock)

/*

K is native representation of k-ordered number both local and global formats.

The Golang struct takes 128-bits but the library effectively uses either 64-bits
or 96-bits. The serialization ensures that right amount of bits is used.
*/
type K struct{ hi, lo uint64 }

/*

ID representation of k-order number
*/
type ID string

/*

...

*/
type Sequence interface {
	Z() K
	L() K
	G() K
	ID() string
}

/*

xxx

*/
type Kx interface {
	// Split number to fractions
	Val() (uint64, uint64)

	ToG()
	ToL()
	Time() int64
	Node() uint64
	Seq()
}
