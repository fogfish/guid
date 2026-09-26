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
// All three are plain, already-resolved values — T reads a field, nothing
// more. An application that wants its own node identity, drift or ticker
// does not configure one of these in place; it derives a new Chrono from one
// with NewClock, see below.
var (
	Clock   = newDefaultClock(unixtime, ForwardTime, seqAscending)
	Unclock = newDefaultClock(inversetime, InverseTime, seqDescending)
	Mock    = &Chrono{order: ForwardTime, drift: driftDefault, mock: true}
)

func newDefaultClock(ticker func() uint64, order TimeOrder, shared *sequence) *Chrono {
	return &Chrono{
		location: defaultNode(),
		drift:    driftDefault,
		order:    order,
		ticker:   ticker,
		seq:      shared,
		advance:  advanceFor(shared, order),
	}
}

// advanceFor picks next or prev according to order — the one place both
// newDefaultClock and clockConfig.build decide it, so the two can never
// disagree about which end of a sequence a given direction draws from.
func advanceFor(seq *sequence, order TimeOrder) func(uint64) uint64 {
	if order == InverseTime {
		return seq.prev
	}
	return seq.next
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

func unixtime() uint64 {
	return uint64(time.Now().UnixNano())
}

func inversetime() uint64 {
	return 0xffffffffffffffff - uint64(time.Now().UnixNano())
}

// Chrono is the concrete Chronos NewClock returns. Clock, Unclock and Mock
// are its three process-wide instances; an application names the type
// itself only when it needs to hold a configured clock in a variable or
// struct field before passing it on, or before deriving further from it.
//
// A Chrono is inert once built: nothing here is ever mutated after NewClock
// returns it, so T needs no synchronization of its own beyond whatever the
// sequence it draws from already provides.
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
	// seq is the sequence this clock actually draws from — the process-wide
	// one Clock/Unclock share, or a private one WithClock/WithSeed/
	// WithCheckpoint requested. NewClock reads it back from a seed so that a
	// clock derived from another shares its sequence by default, the same
	// rule Clock/Unclock themselves follow.
	seq *sequence
	// advance is resolved once, by NewClock, before this value ever exists
	// where anything could call T on it — never lazily, never touched again.
	advance func(uint64) uint64
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

	v := c.advance(c.ticker())
	return v >> bitsSeq << bitsSeqDrift, v & maskSeq
}

// Config is a functional option NewClock applies when deriving a Chrono from
// a seed. See WithDrift, WithNodeID, WithNodeRandom, WithNodeFromEnv,
// WithClock, WithSeed and WithCheckpoint.
type Config func(*clockConfig)

// clockConfig accumulates what NewClock's options ask for before it is
// resolved, once, into the Chrono they describe. It is not exported: an
// application configures a clock by calling NewClock with Config values,
// never by naming this type.
type clockConfig struct {
	location uint64
	drift    Drift
	order    TimeOrder
	mock     bool
	ticker   func() uint64

	// shared is the sequence this clock uses unless some option below
	// forces a private one. It starts as the seed's own sequence — see
	// NewClock — so sharing is inherited by default and privacy is always
	// an explicit opt-in, never the other way around.
	shared *sequence

	hasSeed bool
	seed    uint64

	hasCheckpoint bool
	ckInterval    time.Duration
	ckOut         chan<- uint64
}

// NewClock derives a new Chrono from seed, applying every opt in order and
// resolving the result once before returning it — there is no separate
// build step, and nothing about the result depends on what order opts were
// given: WithSeed and WithCheckpoint each just record intent, and NewClock
// reads all of it back together at the end.
//
// The seed is mandatory. It is usually Clock or Unclock — the two the
// library provides — but can be any Chrono, including one NewClock already
// produced: unless an opt forces privacy (WithClock, WithSeed,
// WithCheckpoint), the result shares seed's own sequence, the same rule
// Clock and Unclock themselves follow. That makes sharing recursive by
// default rather than something only the two globals get.
//
//	c := guid.NewClock(guid.Clock, guid.WithNodeID(0xffffffff))
//	h := guid.NewClock(guid.Clock, guid.WithSeed(restored), guid.WithCheckpoint(2*time.Second, ch))
func NewClock(seed *Chrono, opts ...Config) *Chrono {
	cfg := &clockConfig{
		location: seed.location,
		drift:    seed.drift,
		order:    seed.order,
		mock:     seed.mock,
		ticker:   seed.ticker,
		shared:   seed.seq,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg.build()
}

func (cfg *clockConfig) build() *Chrono {
	if cfg.mock {
		return &Chrono{location: cfg.location, drift: cfg.drift, order: ForwardTime, mock: true}
	}

	seq := cfg.shared
	if seq == nil {
		if cfg.order == InverseTime {
			seq = descending()
		} else {
			seq = &sequence{}
		}
	}

	if cfg.hasSeed {
		seq.seed(cfg.seed)
	}

	advance := advanceFor(seq, cfg.order)
	if cfg.hasCheckpoint {
		advance = withCheckpoint(advance, cfg.ckOut, cfg.ckInterval)
	}

	return &Chrono{
		location: cfg.location,
		drift:    cfg.drift,
		order:    cfg.order,
		ticker:   cfg.ticker,
		seq:      seq,
		advance:  advance,
	}
}

// WithDrift configures ⟨𝒅⟩, the failover budget the k-ordering absorbs.
//
// The ladder has eight rungs, from Drift131us to Drift39h, and defaults to
// Drift275s. A larger drift attributes a longer overlap, a smaller one yields
// a tighter k. Use DriftOf to select the rung that covers a budget expressed
// as a duration:
//
//	guid.NewClock(guid.Clock, guid.WithDrift(guid.Drift17s))
//	guid.NewClock(guid.Clock, guid.WithDrift(guid.DriftOf(45 * time.Second)))
//
// Every clock of a keyspace must be configured with the same drift, otherwise
// values are ordered by their drift rather than by their time.
func WithDrift(drift Drift) Config {
	return func(cfg *clockConfig) { cfg.drift = drift & maskDrift }
}

// WithNodeID explicitly configures ⟨𝒍⟩ spatially unique identifier.
//
// The identifier is truncated to 58 bits, the width X gives it. A value of G
// keeps only its low 32 bits, so an identity meant for both types has to fit
// the narrower one.
func WithNodeID(id uint64) Config {
	return func(cfg *clockConfig) { cfg.location = id & maskNodeX }
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
func WithNodeFromEnv() Config {
	return func(cfg *clockConfig) {
		val, ok := os.LookupEnv("CONFIG_GUID_NODE_ID")
		if !ok || val == "" {
			panic("guid: CONFIG_GUID_NODE_ID is not set")
		}
		cfg.location = nodeFromEnvValue(val)
	}
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
func WithNodeRandom() Config {
	return func(cfg *clockConfig) { cfg.location = randomNode() }
}

// WithClock overrides the timestamp generator with a custom ticker, keeping
// the direction (and so the ascending/descending choice of next/prev) of
// whatever seed NewClock was given — guid.NewClock(guid.Clock, ...) stays
// ascending, guid.NewClock(guid.Unclock, ...) stays descending, and there is
// no separate "descending" option: seeding from Unclock already is one.
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
func WithClock(ticker func() uint64) Config {
	return func(cfg *clockConfig) {
		cfg.ticker = ticker
		cfg.shared = nil
	}
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
func WithSeed(v uint64) Config {
	return func(cfg *clockConfig) {
		cfg.hasSeed = true
		cfg.seed = v
		cfg.shared = nil
	}
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
// WithSeed and WithCheckpoint compose regardless of which is given first —
// NewClock reads every opt's effect back together after all of them have
// run, rather than resolving anything as each one is applied.
func WithCheckpoint(interval time.Duration, out chan<- uint64) Config {
	return func(cfg *clockConfig) {
		cfg.hasCheckpoint = true
		cfg.ckInterval = interval
		cfg.ckOut = out
		cfg.shared = nil
	}
}
