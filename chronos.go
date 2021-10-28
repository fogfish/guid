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
	"crypto/rand"
	"crypto/sha256"
	"io"
	"os"
	"time"
)

/*

defaultChronos is default logical clock used by LID() and GID() functions

If the application needs own default clock e.g. inverse one, it declares own
clock and pair of GID & LID functions.
*/
var defaultChronos Chronos = NewLClock()

/*

NewLClock creates a new instance of Chronos
*/
func NewLClock(opts ...Config) Chronos {
	node := &LClock{}
	ClockUnix()(node)
	NodeRand()(node)

	for _, f := range opts {
		f(node)
	}
	return node
}

/*

NodeID explicitly configures ⟨𝒍⟩ spatially unique identifier
*/
func NodeID(id uint64) Config {
	return func(clock *LClock) {
		clock.location = id & 0x00000000ffffffff
	}
}

/*

NodeFromEnv configures ⟨𝒍⟩ spatially unique identifier using env variable.
  CONFIG_GUID_LOCATION_ID - defines location id as a string
*/
func NodeFromEnv() Config {
	return func(clock *LClock) {
		h := sha256.New()
		h.Write([]byte(os.Getenv("CONFIG_GUID_LOCATION_ID")))
		hash := h.Sum(nil)
		clock.location = uint64(hash[0])<<24 | uint64(hash[1])<<16 | uint64(hash[2])<<8 | uint64(hash[3])
	}
}

/*

NodeRand configures ⟨𝒍⟩ spatially unique identifier using cryptographic random generator
*/
func NodeRand() Config {
	return func(clock *LClock) {
		rander := rand.Reader
		bytes := make([]byte, 8)
		if _, err := io.ReadFull(rander, bytes); err != nil {
			panic(err.Error())
		}

		node := uint64(0x0)
		for i, b := range bytes {
			node = node | uint64(b)<<(64-8*(i+1))
		}
		clock.location = node & 0x00000000ffffffff
	}
}

/*

Clock configures a custom timestamp generator function
*/
func Clock(ticker func() uint64) Config {
	return func(clock *LClock) {
		clock.ticker = ticker
	}
}

/*

ClockUnix configures unix timestamp time.Now().UnixNano() as generator function
*/
func ClockUnix() Config {
	return func(clock *LClock) {
		clock.ticker = unixtime
	}
}

func unixtime() uint64 {
	return uint64(time.Now().UnixNano())
}

/*

ClockInverse configures inverse unix timestamp as generator function
*/
func ClockInverse() Config {
	return func(clock *LClock) {
		clock.ticker = inversetime
	}
}

func inversetime() uint64 {
	return 0xffffffffffffffff - uint64(time.Now().UnixNano())
}
