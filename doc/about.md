# What `guid` guarantees, and where it stops

A plain-language summary of the ordering properties this library provides. The
formal statements, with proofs, are in [proof.md](proof.md); statement (2) is
machine-checked in [proof.lean](proof.lean), at both of the widths the library
offers a node id at. This note is the version you read first.

## The problem

You want identifiers that are (a) unique across many machines, (b) roughly
sorted by creation time, (c) generated without any coordination — no lock, no
consensus, no central allocator — and (d) able to tell you *which* machine
allocated them, by sort position, so that two writers covering the same range
can be told apart.

These fight each other. Sorting by time requires machines to agree on the time,
and machines do not agree on the time — especially machines you do not
administer. Something has to give, and the design's job is to control *exactly
what* gives.

## The layout is the whole design

An identifier is just a number, cut into fields. Comparing two identifiers is
comparing two numbers, so **the field order is the sort order**:

```
   ⟨d⟩       ⟨E⟩          ⟨l⟩         ⟨xₗ⟩      ⟨s⟩
  drift   coarse time    node id    fine time  counter
    3    |  47−D bits  |  32 bits |  D bits  |  14  |
```

The library ships this layout at three widths. They differ only in how much
room is left for the node id — none, 32 bits, or 58 — and everything below is
true of all three.

|          | size | node id | space if assigned                                         | space if random                    |
| -------- | ---- | ------- | --------------------------------------------------------- | ---------------------------------- |
| `guid.L` | 8 B  | —       | unique within one allocator                               | —                                  |
| `guid.G` | 12 B | 32 bit  | up to 2³² ≈ 4.3·10⁹ allocators                            | ≈ 65 000 before collisions matter  |
| `guid.X` | 16 B | 58 bit  | up to 2⁵⁸ allocators, and it **is** an RFC 9562 UUID (v8) | ≈ 5.4·10⁸ before collisions matter |

The "space if random" column is the birthday bound — it only applies when node
ids are drawn from a random generator. Assign them instead (sequential
counter, MAC-derived, or otherwise coordinated) and the birthday paradox does
not apply: the space is the full field width, since collisions come from
double-assignment, not from chance.

The one unusual move: **the node id sits in the middle of the timestamp.**
Coarse time is above it, fine time below it. That single choice produces
everything else:

* Two identifiers in **different coarse-time buckets** → time decides. Order is
  correct, always.
* Two identifiers in the **same bucket** → node id decides. Order is grouped by
  allocator rather than by time.

The second line is the product, not a concession. Inside one bucket every
node's identifiers form a **contiguous run**, and each run is exactly ordered.
A leader fails silently; for some interval two nodes believe they own the same
range. Under a timestamp-primary schema their writes interleave and you need a
side channel to work out who wrote what. Here they land in separate ranges — you
scan a node's contribution, bound the overlap and reconcile, because the key
space records the topology.

Snowflake cannot do this: its machine id sits below the *whole* timestamp, so
grouping survives only within one millisecond. UUIDv7 cannot either — it has no
location field at all. This is a difference in what the layout can express, not
a difference in tuning.

The bucket width is `Δ`, configured with [`WithDrift`](../clock.go#L153) from
a ladder of eight rungs, `guid.Drift131us` … `guid.Drift39h`. Default
`guid.Drift275s` — 274.9 s — and the ladder runs from 131 µs to 39.1 h.
`guid.DriftOf(d)` picks the smallest rung that covers a budget `d`.

**Read `Δ` as a failover budget, not a clock-skew budget.** It has to cover the
interval between a silent failure and the moment the cluster has converged on a
new owner, because that is exactly the interval during which two allocators
write to the same range:

| rung                                   | `Δ`             | covers                                                        |
| -------------------------------------- | --------------- | ------------------------------------------------------------- |
| `Drift131us`                           | 131 µs          | ordering only, no attribution — Snowflake's field order       |
| `Drift2s` – `Drift17s`                 | 2.15 s – 17.2 s | consensus election, gossip, ZooKeeper and Consul sessions     |
| `Drift68s` – `Drift275s` *(default)*   | 68.7 s – 274.9 s | K8s node-NotReady, Kafka session, automated cross-AZ         |
| `Drift1099s` – `Drift4398s`            | 1099 s – 4398 s | paging, cross-region, human-in-the-loop failover              |
| `Drift39h`                             | 39.1 h          | split brain found the next morning, fleets syncing once a day |

Only one rung sits below a second. The window is `Δ + 2ε`, so two rungs differ
in practice only where the wider `Δ` is large against `2ε` — below a second the
clocks decide `W`, not the setting, so further rungs there would differ in name
only. At the floor a run is one tick wide, so that rung buys ordering and no
attribution; at the top, `Δ = 39.1 h` is where the bucket stops discriminating
anything because every value lands in one of them.

It is *also* the clock disagreement the ordering tolerates, which is why one
knob serves both. And that range is not over-generous: the target is not a
managed cluster with datacenter NTP but uncoordinated nodes — hardware with no
RTC, devices behind firewalls that block NTP, VMs resuming from snapshot,
phones returning from airplane mode. Tens of seconds of disagreement is the
distribution there, not a pathology.

> **⟨l⟩ is an ordered field, and assigning it is management plane.** The node
> identity is not an opaque tag that only has to differ — it is a position in an
> ordered space, and the key space ranks by that order. So take it from
> something you already order — ring position, shard number, membership index —
> with [`WithNodeID`](../clock.go#L164), and adjacency in the key space becomes
> adjacency in the topology. This costs nothing at allocation time: ⟨l⟩ is fixed
> once, out of band, wherever membership is already decided, while allocation
> itself stays lock-free and consults nobody. The default
> [`WithNodeRandom`](../clock.go#L197) is a seeding policy for that same ordered
> space — it picks *which* position a node takes, so the result still sorts, but
> arbitrarily, and adjacent ring positions land far apart.

## The three results

### 1. Local identifiers (`guid.L`, 8 bytes) are perfectly sorted

Not "approximately". If one process allocates `a` then `b`, then `a < b`.
Always.

The reason is simpler than you would expect: the process keeps **one counter**,
and the identifier *is* that counter plus a constant. The counter only moves up
— allocating does `+1`. A number that only increases, incremented atomically,
hands out increasing values. That is the whole proof (Theorem 1, §3.5).
The counter is protected from NTP moving the clock backwards. 

> **This used to be false in version 2 of the library.**
> Older versions drew the timestamp and the counter
> from two independent sources. The counter wrapped every 16,384 values, and if
> it wrapped while the timestamp stood still, two identifiers came out
> backwards. The trap was that staying under 16,384 allocations per tick *did
> not help* — what breaks order is crossing a multiple of 16,384, not how many
> you make between crossings. Two allocations numbered 16383 and 16384 invert.
> The fix was to make the timestamp and counter one number, see
> [sequence.go](../sequence.go).

### 2. Global identifiers (`guid.G` 12 bytes, `guid.X` 16 bytes) are k-ordered

The guarantee has a shape worth stating precisely:

> Two identifiers allocated **more than `W = Δ + 2ε` apart** in real time are
> ordered by their allocation time. Two identifiers allocated **closer than
> that** are ordered by ⟨l⟩, then fine time, then counter — an order that need
> not follow allocation time.

where `ε` is your worst clock skew. Note what is *not* being claimed. The order
is never undetermined: identifiers are numbers, comparison is lexicographic on
`(⟨d⟩, ⟨E⟩, ⟨l⟩, ⟨xₗ⟩, ⟨s⟩)`, and every field is a strict order, so any two
values compare the same way every time, for everyone. Inside the window the
order is as total and as reproducible as outside it. What `W` bounds is the
*divergence between that order and real time* — and randomizing ⟨l⟩ does not
weaken this, it only selects which permutation of the nodes the key space ranks
by, once, when the ids are drawn.

Why: the coarse-time field outranks everything below it, and the fields below
it cannot sum large enough to carry into it. So a difference in coarse time can
never be overturned by node id, fine time, or counter. Two clocks that differ
by at most `ε` are guaranteed to land in different buckets once real time has
advanced by `Δ + 2ε`.

"k-ordered" is the index-space version of the same fact: if the cluster makes
`ρ` identifiers per second then `k = ρ·W`, and sorting the stream moves nothing
more than `k` positions.

### 3. Within one node, global identifiers are perfectly sorted too

Fix the node id and the comparison collapses back to case 1. So the useful
mental model is:

> **The global stream is N perfectly sorted streams, concatenated as
> node-id runs per bucket — exactly, not approximately.**

"Concatenated," not "merged": inside one bucket the streams don't interleave at
all — each node's values sit together as one contiguous run, and the runs are
ordered by node id. Across buckets, bucket order always wins. So the whole
global order is a deterministic, reproducible function of the two values being
compared, at every separation — there is no point where it becomes fuzzy or
merely probable. The only thing that is approximate is the relationship
between this order and *real time*: a node's run can fall in a
bucket-and-node-id position that a wall clock would not have predicted — and
`W` is exactly the real-time separation two allocations need before that
cannot happen: closer together than `W`, a later allocation can still sort
before an earlier one; that far apart, never.

That mismatch is not something this library trades away by being imprecise —
it's unavoidable for *any* coordination-free scheme built from timestamps
alone (§5 of [proof.md](proof.md)). Snowflake and UUIDv7 face exactly the same
problem; they just don't tell you when it bit them, because neither has
anything to arbitrate with once two values tie. Snowflake's machine id sits
below the *whole* timestamp, so its tiebreak survives only within one
millisecond, and UUIDv7 has no location field at all — its tie is broken by
bits that carry no meaning. What this layout buys is not a smaller mismatch
window but a **legible** one: when two identifiers do land out of real-time
order, ⟨l⟩ still tells you deterministically which allocator's run is which,
so the disagreement is something you can scan, bound and reconcile instead of
something you can only wonder about.

Every inversion is between nodes, never within one — that is now Lemma 5′, not
just an empirical pattern. If you shard by node, each shard is exactly
ordered, and if a node's contribution to a failover spans multiple buckets, it
shows up as one contiguous run per bucket rather than one run for the
whole incident.

## What this means operationally

| you want to                               | do this                                                          |
| ----------------------------------------- | ---------------------------------------------------------------- |
| a sortable key in a database              | use `L` — 8 bytes, strictly ordered, see below                   |
| tell two overlapping writers apart        | scan each node's run per bucket; runs never interleave           |
| range scan by time                        | widen the range by `W` on both sides                             |
| fully sort the stream                     | buffer `k` entries — but see below                               |
| ordering that tracks real allocation time | stay within one node, or use `L`; across nodes, only outside `W` |
| read a timestamp back                     | `Time()` is accurate to 131 µs, not a precise clock              |

That "ordering that tracks real allocation time" row is worded carefully. The
key order itself is *always* exact and total — Lemma 5 across buckets, Lemma 5′
within one — so "exact ordering" is never actually at risk; what's at risk is
whether that order lines up with the order things really happened in. Reach
for `L`, or stay on one node, only when the second thing is what you need.

**For a database surrogate key, reach for `guid.L`.** It is 8 bytes — half a
UUID, a third smaller than `G`, the width of a `BIGINT` — and it is *strictly*
ordered, not k-ordered: there is no drift window to reason about, because there
is no ⟨l⟩ fraction for time to rank against. Its limit is in the name: it is
unique within the allocator that issued it, so use it where the context already
disambiguates (per-tenant or per-partition key space, a single writer), and use
`G` where it does not.

The sort buffer deserves a warning. `k = ρ·W`, so at 10⁵ identifiers per second
with default settings, `k ≈ 2.8·10⁷` — a 330 MB heap. **`k` grows with your
throughput; `W` does not.** Always reason in the time form ("widen by `W`
seconds"), never the entry form.

## Where it breaks

In rough order of how likely you are to hit it.

**Inversions inside `W` — the normal case, not a bug.** For two allocations
less than `W` apart on different nodes, the earlier one can carry the *larger*
key (Lemma 7); past `W`, never — and never on the same node, at any distance,
regardless of `W` (Corollary 2 / Lemma 5′). This is not corruption or
non-determinism: `Before` still gives the same answer to every reader, every
time (Lemma 1); it's a bounded, reproducible mismatch between key order and
arrival order, not an unsettled comparison. Where it actually shows up:

* A range scan over `[t₁, t₂]` can miss an entry that landed just outside the
  naive boundary, or include one that didn't belong — widen by `W` on both
  sides (see the table above).
* Reconstructing a fully time-ordered stream needs a buffer, not a pass-through
  — `k = ρ·W` entries, per Corollary 1, not zero.
* During a leader hand-over, the two nodes' runs can appear in the "wrong"
  real-time order relative to each other — but `𝒍` still says unambiguously
  which run is whose, so attribution survives even when arrival order does
  not.

None of this compounds: the effect is per-pair, bounded, and tight at exactly
`W − 1` (§4.8) — not a growing or cascading problem.

**Node id collisions — the real limit on cluster size, if you randomize.** In a
`guid.G`, node ids are 32 bits, and the default is to draw them randomly with
[`WithNodeRandom`](../clock.go#L197). By the birthday bound that gives roughly
**65,000 allocators** before collision probability approaches ½ — a property
of random allocation, not of the field width. Two allocators sharing a node
id, in the same bucket, at the same fine time, with the same counter value,
produce *the same identifier*. Uniqueness is assumed, not proven — it is the
one thing here that is not.

The birthday bound goes away if node ids are assigned rather than drawn at
random — a sequential counter, a MAC-derived value, or any other coordinated
scheme — since then a collision requires two allocators to be given the same
id, not merely to guess into the same pool. Assign them explicitly with
[`WithNodeID`](../clock.go#L164) and the usable space is the full 2³² ≈
4.3·10⁹, no birthday discount. If you cannot coordinate assignment and must
stay with random ids, use `guid.X` instead, whose 58 random bits move the
random-allocation bound to about **5.4·10⁸** allocators. That is the reason
the wider type exists; being a UUID is the other one.

**Mixed drift settings — silent and nasty.** The drift code is the *top* field.
Identifiers made with different drift settings sort by their configuration
rather than their time. This is now hard to do by accident — drift lives on the
clock, not on each call — but two clocks built differently still feed one
keyspace if you let them. It shows up as a bimodally distributed key space, not
as an error.

**Underestimated clock skew.** If your real `ε` exceeds what you assumed, `W`
is wider than you budgeted. Nothing fails, nothing errors — you just get more
inversions than planned, because pairs separated by your configured `W` no
longer satisfy Lemma 7's hypothesis at the real `ε`. Degradation here is
graceful, which is also why it can go unnoticed.

Practically: don't guess `ε`, measure it — the worst clock offset actually
observed across your fleet, not a textbook NTP figure, since the library's
target deployments (devices behind NAT, VMs resuming from snapshot, hardware
with no RTC) routinely exceed textbook skew. Then budget margin rather than a
point estimate: pick a rung with [`DriftOf`](../drift.go#L107) using a
tolerance well above your measured `ε`, so `Δ` dominates `W = Δ + 2ε` and a
misestimate in `ε` moves `W` only a little — the default `Drift275s` has two
and a half minutes of `Δ` to absorb a few extra seconds of misjudged skew;
`Drift2s` does not. And because the failure is silent, watch for it rather
than assuming it away: sample `Time()` on incoming identifiers against the
receiver's wall clock, and treat a growing gap as a sign your assumed `ε` no
longer matches reality, before it does.

**Hand-written `Chronos`.** If you implement the clock interface yourself,
`T()` must return a timestamp/counter pair that strictly increases. Return them
from independent sources and you reintroduce the wrapping bug described above.
The library's own clocks get this right by construction.

**Saturation run-ahead.** A loop that allocates and discards, doing nothing
else, makes `Time()` report ahead of the wall clock. Ordering is unaffected,
and the gap closes by itself when the loop stops. Reachable only synthetically
— the threshold is about the cost of the atomic increment itself. §3.6 of
[proof.md](proof.md) has the rates.

**`Diff` is approximate.** It is documented as such. It subtracts fields
independently, so the result is only meaningful when both the time and the
counter of the first operand are larger.

**Global identifiers are not linearizable — and cannot be.** This one is a
limit, not a defect. Any scheme that compares two identifiers using only the
identifiers, with no coordination, cannot produce a total order that agrees
with real time when clocks are unsynchronized. k-ordering is the strongest
guarantee that survives the absence of coordination. If you need a true total
order, you need coordination, and this library is the wrong tool.

## What is proven vs. what is assumed

Worth separating, because the two carry very different weight.

**Proven**, and machine-checked in Lean 4 with no `sorry` and no classical
choice: the bit layout is what the code produces; coarse time cannot be
overturned; the inversion window is `Δ + 2ε`; the k-ordering statement; local
values sort in allocation order.

**Assumed** — these are facts about *your deployment*, and the library cannot
check them: clock skew stays within `ε`; node ids are distinct; every clock in
a keyspace uses the same drift.

And the bound is **tight** (§4.8): there is an explicit two-node execution that
inverts two values `W − 1` apart, so no better constant exists. The only ways
to reduce inversions are a smaller `Δ` or better clock sync — you cannot
analyze your way to a smaller number.

## Terminology

Terms used above and in other notes.

| term                    | meaning                                                                                                                                                                                            | defined / proven in                                                                         |
| ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| **bucket** (⟨E⟩, epoch) | the `Δ`-wide slice of time a value's coarse-time field places it in; two values in different buckets are ordered by bucket alone, regardless of node id                                            | §1.1 of [proof.md](proof.md) (notation); Lemma 5 (epoch dominance)                          |
| **drift**, `Δ`          | the configured bucket width — a failover budget, not a clock-skew budget — chosen from the ladder via `WithDrift`/`DriftOf`                                                                        | §1.1 of [proof.md](proof.md); "The bucket width is `Δ`" above                               |
| **node id**, `⟨l⟩`      | the allocator's identity: an element of an ordered space (assigned or randomly seeded), ranking values within one bucket                                                                           | "The one unusual move" above; §1.2 of [proof.md](proof.md)                                  |
| **run**                 | a maximal contiguous stretch, *within one bucket*, of values belonging to one node; runs from different nodes never interleave inside a bucket, and a run does not extend across a bucket boundary | "The one unusual move" above; Corollary 2 / Lemma 5′ of [proof.md](proof.md)                |
| **inversion**           | a pair of allocations where the one that happened later in real time sorts *before* the one that happened earlier                                                                                  | Lemma 7 of [proof.md](proof.md); "Inversions inside `W`" above                              |
| **`W`**                 | the real-time separation, `Δ + 2ε`, beyond which two allocations can never invert; tight, not just an upper bound (§4.8)                                                                           | Lemma 7 of [proof.md](proof.md)                                                             |
| **`k`**                 | the index-space form of `W`: at peak rate `ρ`, `k = ρ·W` values can appear in one window, so a full re-sort needs a buffer of `k` entries                                                          | Theorem 2 / Corollary 1 of [proof.md](proof.md); "The sort buffer deserves a warning" above |
| **overlap**             | the real-time interval during which two owners concurrently accept writes for the same key range — the split-brain or hand-over scenario a run's contiguity is meant to make attributable          | [vnode.md](vnode.md), "The shape of the problem"                                            |
| **ring order**, `<ₖ`    | the order on `⟨l⟩` cut at a ring position `k` rather than at 0, so the owner of `k` is the least element; `Before` is the member of that family cut at 0, which is why it misfires off the origin  | [ring.go](../ring.go); [vnode.md](vnode.md), "Use the vnode token"                          |
