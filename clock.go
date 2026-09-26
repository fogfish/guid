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
	"sync/atomic"
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

// Clock, Unclock and Mock are the process-wide instances of the logical
// clock every application starts from — most never need anything else.
//
//   - Clock allocates from the ascending unix-time domain, ⟨𝒕⟩ = UnixNano.
//   - Unclock allocates from the descending domain, ⟨𝒕⟩ = MaxUint64 − UnixNano,
//     so that recently allocated values sort first.
//   - Mock is deterministic, pinning every allocation to ⟨0,0⟩; it is for
//     tests that assert on a fixed value and must not be used in production.
//
// Clock and Unclock take their ⟨𝒍⟩ from defaultNode: CONFIG_GUID_NODE_ID when
// the deployment sets it, so every process that sets the same value gets the
// same stable node identity, and a random one otherwise. Mock's is ⟨0⟩.
//
// Each is configurable in place, in the sense that every WithXXX method
// returns a new, independently configured clock rather than mutating the
// receiver — so an application can derive its own clock straight from one of
// these three (guid.Clock.WithNodeID(x)) without either global ever
// reflecting that configuration back to any other caller. See WithSeed and
// WithCheckpoint for the one option that also gives the derived clock a
// private ⟨𝒕,𝒔⟩ sequence rather than the one Clock/Unclock share process-wide.
var (
	Clock   = newDefaultClock(unixtime, ForwardTime, seqAscending)
	Unclock = newDefaultClock(inversetime, InverseTime, seqDescending)
	Mock    = &Chrono{order: ForwardTime, drift: driftDefault, mock: true, advance: new(atomic.Pointer[func(uint64) uint64])}
)

func newDefaultClock(ticker func() uint64, order TimeOrder, shared *sequence) *Chrono {
	return &Chrono{
		location: defaultNode(),
		drift:    driftDefault,
		order:    order,
		ticker:   ticker,
		shared:   shared,
		advance:  new(atomic.Pointer[func(uint64) uint64]),
	}
}

func randomNode() uint64 {
	rander := rand.Reader
	raw := make([]byte, 8)
	if _, err := io.ReadFull(rander, raw); err != nil {
		panic(err.Error())
	}

	node := uint64(0x0)
	for i, v := range raw {
		node = node | uint64(v)<<(64-8*(i+1))
	}
	return node & maskNodeX
}

// Chrono is the concrete Chronos every WithXXX method builds and returns.
// Clock, Unclock and Mock are its three process-wide instances; an
// application names the type itself only when it needs to hold a configured
// clock in a variable or struct field before passing it on.
type Chrono struct {
	// Spatially unique identifier ⟨𝒍⟩
	location uint64
	// ⟨𝒅⟩ drift, the rung of the ladder, see Drift
	drift Drift
	// ⟨𝒐⟩ direction of the time domain
	order TimeOrder
	// mock pins T to ⟨0,0⟩ unconditionally, see Mock
	mock bool
	// Monotonically increasing logical clock ⟨𝒕⟩ generator
	ticker func() uint64

	// shared is the process-wide sequence this clock uses unless a private
	// one is requested — nil once WithClock, WithSeed or WithCheckpoint has
	// been called anywhere in this clock's fork chain.
	shared *sequence

	hasSeed bool
	seed    uint64

	hasCheckpoint bool
	ckInterval    time.Duration
	ckOut         chan<- uint64

	// advance caches the outcome of resolve, below: the wrapped advance
	// function built from whatever combination of WithSeed/WithCheckpoint
	// this clock accumulated, regardless of the order they were called in.
	// nil until first resolved. A pointer to an atomic.Pointer, not a plain
	// one, for the same reason as sequence's fields further down this file:
	// fork copies a Chrono by value, and an atomic type must never be
	// copied after use — fork always replaces it with a fresh one, but a
	// value field would still make that copy look, to go vet and to a
	// future reader, like copying state that is live elsewhere.
	advance *atomic.Pointer[func(uint64) uint64]
}

func (c *Chrono) Node() uint64 { return c.location }

func (c *Chrono) Drift() Drift { return c.drift }

func (c *Chrono) Order() TimeOrder { return c.order }

// T allocates the ⟨𝒕,𝒔⟩ fraction of a k-ordered value.
//
// ⟨𝒕⟩ and ⟨𝒔⟩ are allocated together, as one atomic step, so that the pair
// strictly increases with every call and values are ordered exactly as they
// are allocated. See sequence for why the two cannot be drawn independently.
func (c *Chrono) T() (uint64, uint64) {
	if c.mock {
		return 0, 0
	}

	advance := c.advance.Load()
	if advance == nil {
		advance = c.resolve()
	}

	v := (*advance)(c.ticker())
	return v >> bitsSeq << bitsSeqDrift, v & maskSeq
}

// resolve settles this clock's sequence and advance function exactly once,
// the first time it is needed, from whatever WithClock/WithSeed/WithCheckpoint
// accumulated on the fork chain that produced this value. Resolving lazily
// rather than at each WithXXX call is what lets those three compose in any
// order: whichever is called last does not clobber what an earlier one in
// the same chain set up, because none of them touch the sequence directly —
// they only record intent, and resolve reads all of it together.
//
// Two goroutines racing here may both build a candidate — each seeding its
// own throwaway private sequence — before one wins the CompareAndSwap; the
// loser's sequence was never handed to any allocator, so discarding it is
// exactly as safe as sequence.seed's contract requires. There is no lock:
// the redundant work of losing the race is cheaper than blocking on one.
func (c *Chrono) resolve() *func(uint64) uint64 {
	seq := c.shared
	if seq == nil {
		if c.order == InverseTime {
			seq = descending()
		} else {
			seq = &sequence{}
		}
	}

	if c.hasSeed {
		seq.seed(c.seed)
	}

	advance := seq.next
	if c.order == InverseTime {
		advance = seq.prev
	}

	if c.hasCheckpoint {
		advance = withCheckpoint(advance, c.ckOut, c.ckInterval)
	}

	if c.advance.CompareAndSwap(nil, &advance) {
		return &advance
	}
	return c.advance.Load()
}

// fork copies the receiver so that every WithXXX method returns a new clock
// instead of mutating the one it was called on. This is what makes
// configuring guid.Clock or guid.Unclock directly safe: neither global is
// ever touched, no matter how a caller configures the value it gets back.
func (c *Chrono) fork() *Chrono {
	cp := *c
	cp.advance = new(atomic.Pointer[func(uint64) uint64])
	return &cp
}

// WithDrift configures ⟨𝒅⟩, the failover budget the k-ordering absorbs.
//
// The ladder has eight rungs, from Drift131us to Drift39h, and defaults to
// Drift275s. A larger drift attributes a longer overlap, a smaller one yields
// a tighter k. Use DriftOf to select the rung that covers a budget expressed
// as a duration:
//
//	guid.Clock.WithDrift(guid.Drift17s)
//	guid.Clock.WithDrift(guid.DriftOf(45 * time.Second))
//
// Every clock of a keyspace must be configured with the same drift, otherwise
// values are ordered by their drift rather than by their time.
func (c *Chrono) WithDrift(drift Drift) *Chrono {
	cp := c.fork()
	cp.drift = drift & maskDrift
	return cp
}

// WithNodeID explicitly configures ⟨𝒍⟩ spatially unique identifier.
//
// The identifier is truncated to 58 bits, the width X gives it. A value of G
// keeps only its low 32 bits, so an identity meant for both types has to fit
// the narrower one.
func (c *Chrono) WithNodeID(id uint64) *Chrono {
	cp := c.fork()
	cp.location = id & maskNodeX
	return cp
}

// WithNodeFromEnv configures ⟨𝒍⟩ spatially unique identifier using env variable.
//
// CONFIG_GUID_NODE_ID - defines location id as a string
//
// The identity is the leading bytes of the SHA-256 of the variable, taken to
// the 58 bits X gives ⟨𝒍⟩. The four bytes a G reads are kept at the bottom of
// the identity rather than at its top, so that the G a given variable names
// does not depend on how wide the clock's node field happens to be.
//
// The variable must be set and defined, otherwise it panics. See defaultNode
// for the fallback Clock and Unclock use instead of panicking.
func (c *Chrono) WithNodeFromEnv() *Chrono {
	val, ok := os.LookupEnv("CONFIG_GUID_NODE_ID")
	if !ok || val == "" {
		panic("guid: CONFIG_GUID_NODE_ID is not set")
	}

	cp := c.fork()
	cp.location = nodeFromEnvValue(val)
	return cp
}

func nodeFromEnvValue(val string) uint64 {
	h := sha256.New()
	h.Write([]byte(val))
	hash := h.Sum(nil)

	node := uint64(hash[0])<<24 | uint64(hash[1])<<16 | uint64(hash[2])<<8 | uint64(hash[3])
	node |= uint64(hash[4])<<48 | uint64(hash[5])<<40 | uint64(hash[6])<<32

	return node & maskNodeX
}

// defaultNode is the ⟨𝒍⟩ Clock and Unclock start from: CONFIG_GUID_NODE_ID
// when it is set, so that every process of a deployment that assigns it
// gets the same, stable node identity without each one calling
// WithNodeFromEnv itself; a random identity otherwise, exactly as
// WithNodeRandom would produce. Unlike WithNodeFromEnv, an unset variable is
// not an error here — falling back is the whole point of a default.
func defaultNode() uint64 {
	if val, ok := os.LookupEnv("CONFIG_GUID_NODE_ID"); ok && val != "" {
		return nodeFromEnvValue(val)
	}
	return randomNode()
}

// WithNodeRandom configures ⟨𝒍⟩ spatially unique identifier using cryptographic random generator.
//
// The identity is 58 random bits, which is what makes the allocator
// coordinator-free: the birthday bound is ≈ 5.4·10⁸ allocators for X. A G
// keeps only 32 of those bits and its bound is ≈ 6.5·10⁴ — the number to plan
// against if the keyspace is G rather than X.
//
// Clock and Unclock already carry ⟨𝒍⟩ from defaultNode — CONFIG_GUID_NODE_ID
// when it is set, random otherwise; WithNodeRandom is for re-rolling a random
// one explicitly, overriding whichever of the two a derived value inherited.
func (c *Chrono) WithNodeRandom() *Chrono {
	cp := c.fork()
	cp.location = randomNode()
	return cp
}

func unixtime() uint64 {
	return uint64(time.Now().UnixNano())
}

func inversetime() uint64 {
	return 0xffffffffffffffff - uint64(time.Now().UnixNano())
}

// WithClock overrides the timestamp generator with a custom ticker, keeping
// the direction (and so the ascending/descending choice of next/prev) of
// whatever clock it is called on — guid.Clock.WithClock(...) is ascending,
// guid.Unclock.WithClock(...) is descending, and there is no separate
// "descending" constructor: forking from Unclock already is one.
//
// The generator should be non-decreasing: it defines the direction in which
// allocated values sort, and a wandering generator costs Epoch its accuracy,
// see below.
//
// "Should" rather than "must": ⟨𝒕,𝒔⟩ ordering itself does not depend on it.
// Every clock — this one included — allocates through the same coupled
// sequence (Algorithm 1 of doc/proof.md §3.2), which every call advances by
// one, unconditionally; a generator reading may raise the sequence but can
// never lower it, so values allocated from one Chronos strictly increase in
// the order they were allocated regardless of what the generator returns —
// repeating, decreasing, or constant. §3 and §7 of doc/proof.md prove this
// for the general case. What a non-monotonic generator costs is accuracy,
// not order: Epoch and Time report the sequence's high water mark until the
// generator catches back up to it, which for a generator that never
// decreases is immediately.
//
// The generator is assumed to yield unix nanoseconds. Epoch reads ⟨𝒕⟩ back on
// that assumption; a generator in any other unit allocates ordered values but
// does not report a meaningful wall clock time.
//
// WithClock always gives the result its own private ⟨𝒕,𝒔⟩ sequence, since the
// library cannot know whether a custom generator shares a time domain with
// any other clock. Values allocated from two such clocks are therefore
// unique only if the clocks also carry distinct ⟨𝒍⟩ node identity.
func (c *Chrono) WithClock(ticker func() uint64) *Chrono {
	cp := c.fork()
	cp.ticker = ticker
	cp.shared = nil
	return cp
}

// WithSeed configures the sequence's initial ⟨𝒕,𝒔⟩ high-water mark instead of
// starting from zero, so a process resumes at or above where a previous one
// — persisted via WithCheckpoint — left off, rather than resetting to zero.
// That gap is what lets a restart coinciding with a backward clock step (an
// NTP correction, almost always — not a DST change, which UnixNano never
// sees) violate k-ordering: the in-process ratchet that already tolerates a
// backward-stepping ticker (see WithClock) does not survive the process that
// held it.
//
// The value is opaque: it is whatever WithCheckpoint delivered, fed back
// unchanged, and it is only meaningful for a clock built with the same Drift
// and time domain as the one that produced it. A seed at or below the
// sequence's own first reading has no effect — T applies the same max(...)
// ratchet sequence.go already uses — so an absent, zero, or stale seed is
// always safe, merely non-optimal.
//
// WithSeed always gives the result its own private sequence, never the
// process-wide one Clock/Unclock otherwise share: seeding is a deliberate,
// local override and must not silently move the floor every other clock of
// the shared default allocates from. This, like WithCheckpoint, is an option
// for an operator who has already decided to take on that responsibility —
// most applications never need either.
func (c *Chrono) WithSeed(v uint64) *Chrono {
	cp := c.fork()
	cp.shared = nil
	cp.hasSeed = true
	cp.seed = v
	return cp
}

// WithCheckpoint arranges for the clock's ⟨𝒕,𝒔⟩ high-water mark to be sent to
// out on genuine forward progress of the sequence, throttled to at most once
// per interval. The application drains out at its own pace — its own
// goroutine, its own schedule — and persists whatever it last received: a
// file, a KV store, a row of the database the identifiers themselves land
// in. Feeding that value back through WithSeed on the next start is the
// other half of the pair.
//
// The send is non-blocking: a consumer that is not ready simply misses that
// value. Nothing is lost by this that matters, since only the highest value
// the application ever sees is needed to seed a future restart — out should
// be buffered (a capacity of 1 is enough) so a slow-starting consumer does
// not miss the first checkpoint, but no amount of buffering changes the
// guarantee, which is "eventually delivers the latest", not "delivers
// every value".
//
// Like WithSeed, WithCheckpoint always forces a private sequence, so what it
// reports is exactly the clock the caller configured, never a value folded
// in from every other clock sharing a time domain's default sequence.
// WithSeed and WithCheckpoint compose regardless of which is called first —
// guid.Clock.WithSeed(v).WithCheckpoint(iv, ch) and
// guid.Clock.WithCheckpoint(iv, ch).WithSeed(v) resolve to the same clock —
// because neither touches the sequence itself until first use; they only
// record what to do once, at that point, together.
func (c *Chrono) WithCheckpoint(interval time.Duration, out chan<- uint64) *Chrono {
	cp := c.fork()
	cp.shared = nil
	cp.hasCheckpoint = true
	cp.ckInterval = interval
	cp.ckOut = out
	return cp
}
