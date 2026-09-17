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
	"crypto/rand"
	"crypto/sha256"
	"io"
	"os"
	"time"
)

// Chronos is an abstraction of logical clock used by library.
type Chronos interface {
	// Spatially unique identifier ⟨𝒍⟩ of ID allocator so called node location
	L() uint64
	// Monotonically increasing logical clock ⟨𝒕⟩
	T() (uint64, uint64)
}

// Clock is global default instance of logical clock
//
// If the application needs own default clock e.g. inverse one, it declares own
// clock and pair of GID & LID functions.
var Clock Chronos = NewClock()

// Logical Clock Type, the default one
type clock struct {
	// Spatially unique identifier ⟨𝒍⟩
	location uint64
	// Monotonically increasing logical clock ⟨𝒕⟩ generator
	ticker func() uint64
	// Allocator of the coupled ⟨𝒕,𝒔⟩ pair, see sequence
	advance func(uint64) uint64
	// Decoupled ⟨𝒔⟩ generator, engaged only by WithUnique
	unique func() uint64
}

func (clock clock) L() uint64 { return clock.location }

// T allocates the ⟨𝒕,𝒔⟩ fraction of a k-ordered value.
//
// ⟨𝒕⟩ and ⟨𝒔⟩ are allocated together, as one atomic step, so that the pair
// strictly increases with every call and values are ordered exactly as they
// are allocated. See sequence for why the two cannot be drawn independently.
func (clock clock) T() (uint64, uint64) {
	if clock.unique != nil {
		return clock.ticker(), clock.unique()
	}

	v := clock.advance(clock.ticker())
	return v >> bitsSeq << bitsSeqDrift, v & maskSeq
}

// Creates instance of logical clock
func NewClock(opts ...Config) Chronos {
	clock := &clock{}
	defopt := []Config{WithClockUnix(), WithNodeRandom()}

	for _, opt := range append(defopt, opts...) {
		opt(clock)
	}
	return clock
}

// Create mock instance of logical clock
func NewClockMock(opts ...Config) Chronos {
	clock := &clock{
		location: 0,
		ticker:   func() uint64 { return 0 },
		unique:   func() uint64 { return 0 },
	}

	for _, opt := range opts {
		opt(clock)
	}
	return clock
}

// Config option of default logical clock behavior.
// Config options allows to define custom strategies to generate
// ⟨𝒍⟩ location or ⟨𝒕⟩ timestamp.
type Config func(*clock)

// WithNodeID explicitly configures ⟨𝒍⟩ spatially unique identifier
func WithNodeID(id uint64) Config {
	return func(clock *clock) {
		clock.location = id & 0x00000000ffffffff
	}
}

// WithNodeFromEnv configures ⟨𝒍⟩ spatially unique identifier using env variable.
//
// CONFIG_GUID_NODE_ID - defines location id as a string
func WithNodeFromEnv() Config {
	return func(clock *clock) {
		h := sha256.New()
		h.Write([]byte(os.Getenv("CONFIG_GUID_NODE_ID")))
		hash := h.Sum(nil)
		clock.location = uint64(hash[0])<<24 | uint64(hash[1])<<16 | uint64(hash[2])<<8 | uint64(hash[3])
	}
}

// WithNodeRandom configures ⟨𝒍⟩ spatially unique identifier using cryptographic random generator
func WithNodeRandom() Config {
	return func(clock *clock) {
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

// WithClock configures a custom timestamp generator function.
//
// The generator must be non-decreasing: it defines an ascending time domain,
// the direction in which allocated values sort. Use WithClockDescending for a
// generator that runs backwards.
//
// Each clock built with WithClock owns a private ⟨𝒕,𝒔⟩ sequence, since the
// library cannot know whether a custom generator shares a time domain with any
// other clock. Values allocated from two such clocks are therefore unique only
// if the clocks also carry distinct ⟨𝒍⟩ node identity.
func WithClock(ticker func() uint64) Config {
	return func(clock *clock) {
		seq := &sequence{}
		clock.ticker = ticker
		clock.advance = seq.next
		clock.unique = nil
	}
}

// WithClockDescending configures a custom timestamp generator that runs
// backwards, so that recently allocated values sort before older ones. It is
// the custom generator counterpart of WithClockInverse.
func WithClockDescending(ticker func() uint64) Config {
	return func(clock *clock) {
		seq := descending()
		clock.ticker = ticker
		clock.advance = seq.prev
		clock.unique = nil
	}
}

// WithClockUnix configures unix timestamp time.Now().UnixNano() as generator function
func WithClockUnix() Config {
	return func(clock *clock) {
		clock.ticker = unixtime
		clock.advance = seqAscending.next
		clock.unique = nil
	}
}

func unixtime() uint64 {
	return uint64(time.Now().UnixNano())
}

// WithClockInverse configures inverse unix timestamp as generator function,
// so that recently allocated values sort before older ones.
func WithClockInverse() Config {
	return func(clock *clock) {
		clock.ticker = inversetime
		clock.advance = seqDescending.prev
		clock.unique = nil
	}
}

func inversetime() uint64 {
	return 0xffffffffffffffff - uint64(time.Now().UnixNano())
}

// WithUnique configures a generator for ⟨𝒔⟩ that is independent of ⟨𝒕⟩.
//
// Deprecated: the library allocates ⟨𝒕⟩ and ⟨𝒔⟩ as one atomic pair, which is
// what makes values sort in allocation order. Supplying ⟨𝒔⟩ separately opts out
// of that coupling: unless the generator is itself monotone and never folds
// back while ⟨𝒕⟩ stands still, values allocated within the same 2¹⁷ nanosecond
// tick can sort in the opposite order to their allocation. It remains available
// for tests and for applications that need a fixed ⟨𝒔⟩.
//
// The option must be applied after WithClock, WithClockUnix, WithClockInverse
// or WithClockDescending, each of which re-engages the coupled allocation.
func WithUnique(unique func() uint64) Config {
	return func(clock *clock) {
		clock.unique = unique
	}
}
