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

Clock is default logical clock

If the application needs own default clock e.g. inverse one, it declares own
clock and pair of GID & LID functions.
*/
var Clock Chronos = NewLClock()

/*

NewLClock creates a new instance of Chronos
*/
func NewLClock(opts ...Config) Chronos {
	node := &LClock{}
	ConfClockUnix()(node)
	ConfNodeRand()(node)

	for _, f := range opts {
		f(node)
	}
	return node
}

/*

ConfNodeID explicitly configures ⟨𝒍⟩ spatially unique identifier
*/
func ConfNodeID(id uint64) Config {
	return func(clock *LClock) {
		clock.location = id & 0x00000000ffffffff
	}
}

/*

ConfNodeFromEnv configures ⟨𝒍⟩ spatially unique identifier using env variable.
  CONFIG_GUID_NODE_ID - defines location id as a string
*/
func ConfNodeFromEnv() Config {
	return func(clock *LClock) {
		h := sha256.New()
		h.Write([]byte(os.Getenv("CONFIG_GUID_NODE_ID")))
		hash := h.Sum(nil)
		clock.location = uint64(hash[0])<<24 | uint64(hash[1])<<16 | uint64(hash[2])<<8 | uint64(hash[3])
	}
}

/*

ConfNodeRand configures ⟨𝒍⟩ spatially unique identifier using cryptographic random generator
*/
func ConfNodeRand() Config {
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

ConfClock configures a custom timestamp generator function
*/
func ConfClock(ticker func() uint64) Config {
	return func(clock *LClock) {
		clock.ticker = ticker
		clock.unique = uniqueInt
	}
}

/*

ConfClockUnix configures unix timestamp time.Now().UnixNano() as generator function
*/
func ConfClockUnix() Config {
	return func(clock *LClock) {
		clock.ticker = unixtime
		clock.unique = uniqueInt
	}
}

func unixtime() uint64 {
	return uint64(time.Now().UnixNano())
}

/*

ConfClockInverse configures inverse unix timestamp as generator function
*/
func ConfClockInverse() Config {
	return func(clock *LClock) {
		clock.ticker = inversetime
		clock.unique = inverseInt
	}
}

func inversetime() uint64 {
	return 0xffffffffffffffff - uint64(time.Now().UnixNano())
}

/*

TimeUnix convers ⟨𝒕⟩ timestamp fraction from identifier as unix timestamp
*/
func TimeUnix(uid K) time.Time {
	return time.Unix(0, int64(Time(uid)))
}

/*

TimeInverse convers ⟨𝒕⟩ timestamp fraction from identifier as unix timestamp
*/
func TimeInverse(uid K) time.Time {
	t := 0xffffffffffffffff - Time(uid)
	return time.Unix(0, int64(t))
}
