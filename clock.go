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

// TimeOrder is the direction of the time domain a clock allocates from, the
// way its ⟨𝒕⟩ fraction moves as real time advances.
//
// The direction is a decision about keyspace layout, not a fact about the
// events being identified: the same instant is the same instant whichever way
// the index is laid out. Epoch therefore reports wall clock time regardless of
// it, while the sort order of values follows it.
type TimeOrder int

const (
	// ForwardTime domain, ⟨𝒕⟩ = UnixNano. Older values sort first.
	ForwardTime TimeOrder = iota
	// InverseTime domain, ⟨𝒕⟩ = MaxUint64 − UnixNano. Recent values sort
	// first, e.g. so that a range scan returns the newest rows without a
	// reverse.
	InverseTime
)

// Chronos is an abstraction of logical clock used by library.
type Chronos interface {
	// Spatially unique identifier ⟨𝒍⟩ of ID allocator so called node location
	//
	// The identity is carried at the 58 bits X gives it, the widest of the
	// types. G truncates it to its own 32 bits when it stamps a value, so a
	// clock serves both without being configured twice.
	Node() uint64
	// Allocates the coupled ⟨𝒕,𝒔⟩ fraction of a k-ordered value
	T() (uint64, uint64)
	// ⟨𝒅⟩ drift, the rung of the ladder the clock allocates with.
	//
	// The drift decides how much clock disagreement the k-ordering tolerates
	// and it must be the same for every value of a keyspace. It is a property
	// of the clock so that a process cannot vary it per allocation.
	Drift() Drift
	// ⟨𝒐⟩ direction of the time domain.
	//
	// Like the drift it must be the same for every value of a keyspace, and it
	// is a property of the clock so that a process cannot vary it per
	// allocation. It is needed to place a wall clock instant into the domain,
	// see L.FromTime and G.FromTime; reading ⟨𝒕⟩ back does not need it, see
	// Epoch.
	Order() TimeOrder
}

// Clock is global default instance of logical clock
//
// If the application needs own default clock e.g. inverse one, it declares own
// instance of Chronos and passes it to NewG or NewL.
var Clock Chronos = NewClock()

// Logical Clock Type, the default one
type clock struct {
	// Spatially unique identifier ⟨𝒍⟩
	location uint64
	// Monotonically increasing logical clock ⟨𝒕⟩ generator
	ticker func() uint64
	// Allocator of the coupled ⟨𝒕,𝒔⟩ pair, see sequence
	advance func(uint64) uint64
	// ⟨𝒅⟩ drift, the rung of the ladder, see Drift
	drift Drift
	// ⟨𝒐⟩ direction of the time domain, fixed by the WithClock* option
	order TimeOrder
}

func (clock clock) Node() uint64 { return clock.location }

func (clock clock) Drift() Drift { return clock.drift }

func (clock clock) Order() TimeOrder { return clock.order }

// T allocates the ⟨𝒕,𝒔⟩ fraction of a k-ordered value.
//
// ⟨𝒕⟩ and ⟨𝒔⟩ are allocated together, as one atomic step, so that the pair
// strictly increases with every call and values are ordered exactly as they
// are allocated. See sequence for why the two cannot be drawn independently.
func (clock clock) T() (uint64, uint64) {
	v := clock.advance(clock.ticker())
	return v >> bitsSeq << bitsSeqDrift, v & maskSeq
}

// Creates instance of logical clock
func NewClock(opts ...Config) Chronos {
	clock := &clock{}
	defopt := []Config{WithClockUnix(), WithNodeRandom(), WithDrift(driftDefault)}

	for _, opt := range append(defopt, opts...) {
		opt(clock)
	}
	return clock
}

// NewClockMock creates a deterministic instance of logical clock. It pins the
// ⟨𝒕,𝒔⟩ pair to ⟨0,0⟩, so that every value it allocates is identical. The mock
// is intended for tests that assert on a fixed value; it allocates nothing
// unique and must not be used in production.
func NewClockMock(opts ...Config) Chronos {
	clock := &clock{
		location: 0,
		ticker:   func() uint64 { return 0 },
		advance:  func(uint64) uint64 { return 0 },
		drift:    driftDefault,
		order:    ForwardTime,
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

// WithDrift configures ⟨𝒅⟩, the clock disagreement the k-ordering tolerates.
//
// The ladder has eight rungs, from Drift1ms to Drift4398s, and defaults to
// Drift275s. A larger drift tolerates more skew, a smaller one yields a
// tighter k. Use DriftOf to select the rung that covers a tolerance expressed
// as a duration:
//
//	guid.NewClock(guid.WithDrift(guid.Drift16ms))
//	guid.NewClock(guid.WithDrift(guid.DriftOf(5 * time.Second)))
//
// Every clock of a keyspace must be configured with the same drift, otherwise
// values are ordered by their drift rather than by their time.
func WithDrift(drift Drift) Config {
	return func(clock *clock) {
		clock.drift = drift & maskDrift
	}
}

// WithNodeID explicitly configures ⟨𝒍⟩ spatially unique identifier.
//
// The identifier is truncated to 58 bits, the width X gives it. A value of G
// keeps only its low 32 bits, so an identity meant for both types has to fit
// the narrower one.
func WithNodeID(id uint64) Config {
	return func(clock *clock) {
		clock.location = id & maskNodeX
	}
}

// WithNodeFromEnv configures ⟨𝒍⟩ spatially unique identifier using env variable.
//
// CONFIG_GUID_NODE_ID - defines location id as a string
//
// The identity is the leading bytes of the SHA-256 of the variable, taken to
// the 58 bits X gives ⟨𝒍⟩. The four bytes a G reads are kept at the bottom of
// the identity rather than at its top, so that the G a given variable names
// does not depend on how wide the clock's node field happens to be.
func WithNodeFromEnv() Config {
	return func(clock *clock) {
		h := sha256.New()
		h.Write([]byte(os.Getenv("CONFIG_GUID_NODE_ID")))
		hash := h.Sum(nil)

		node := uint64(hash[0])<<24 | uint64(hash[1])<<16 | uint64(hash[2])<<8 | uint64(hash[3])
		node |= uint64(hash[4])<<48 | uint64(hash[5])<<40 | uint64(hash[6])<<32

		clock.location = node & maskNodeX
	}
}

// WithNodeRandom configures ⟨𝒍⟩ spatially unique identifier using cryptographic random generator.
//
// The identity is 58 random bits, which is what makes the allocator
// coordinator-free: the birthday bound is ≈ 5.4·10⁸ allocators for X. A G
// keeps only 32 of those bits and its bound is ≈ 6.5·10⁴ — the number to plan
// against if the keyspace is G rather than X.
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
		clock.location = node & maskNodeX
	}
}

// WithClock configures a custom timestamp generator function.
//
// The generator must be non-decreasing: it defines an ascending time domain,
// the direction in which allocated values sort. Use WithClockDescending for a
// generator that runs backwards.
//
// The generator is assumed to yield unix nanoseconds. Epoch reads ⟨𝒕⟩ back on
// that assumption; a generator in any other unit allocates ordered values but
// does not report a meaningful wall clock time.
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
		clock.order = ForwardTime
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
		clock.order = InverseTime
	}
}

// WithClockUnix configures unix timestamp time.Now().UnixNano() as generator function
func WithClockUnix() Config {
	return func(clock *clock) {
		clock.ticker = unixtime
		clock.advance = seqAscending.next
		clock.order = ForwardTime
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
		clock.order = InverseTime
	}
}

func inversetime() uint64 {
	return 0xffffffffffffffff - uint64(time.Now().UnixNano())
}
