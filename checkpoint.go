//
//  Copyright 2012 Dmitry Kolesnikov, All Rights Reserved
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package guid

import (
	"sync/atomic"
	"time"
)

// checkpointer makes a snapshot of a sequence's high water mark and delivers
// it to an application-owned channel on genuine forward progress, throttled to
// at most once per interval.
//
// See WithCheckpoint. The application is responsible to own
// the channel, drain its values, and to persist them in a durable store.
type checkpointer struct {
	advance  func(uint64) uint64
	out      chan<- uint64
	interval int64 // nanoseconds

	// lastTick is the tick portion of 𝑽 last seen to have changed; used to
	// detect genuine forward progress rather than sampling on a bare timer.
	lastTick atomic.Uint64
	// lastFire is the UnixNano of the last delivered checkpoint; throttles
	// delivery to at most once per interval.
	lastFire atomic.Int64
}

func withCheckpoint(advance func(uint64) uint64, out chan<- uint64, interval time.Duration) func(uint64) uint64 {
	c := &checkpointer{advance: advance, out: out, interval: interval.Nanoseconds()}
	return c.next
}

// next advances the wrapped sequence and, if the tick portion of the result
// has moved since the last call, tries to deliver a checkpoint. Exactly one
// caller among any racing to observe the same tick transition wins the
// attempt, via the CompareAndSwap on lastTick; the interval throttle is then
// applied on top of that. The send to out is non-blocking (see
// ClockBuilder.WithCheckpoint): a full or undrained channel simply misses
// this value.
func (c *checkpointer) next(t uint64) uint64 {
	v := c.advance(t)

	tick := v &^ maskSeq
	if prev := c.lastTick.Load(); tick != prev && c.lastTick.CompareAndSwap(prev, tick) {
		now := time.Now().UnixNano()
		if last := c.lastFire.Load(); now-last >= c.interval && c.lastFire.CompareAndSwap(last, now) {
			select {
			case c.out <- v:
			default:
			}
		}
	}

	return v
}
