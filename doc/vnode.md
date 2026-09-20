# Deploying `guid` on a consistent hashing ring

A deployment note for the use case the layout was designed around: virtual
nodes on a ring, identifiers allocated without coordination, and an interval of
split brain or partial failure during which two owners write to one range.

It assumes you have read [short.md](short.md), which states what the schema
guarantees. This note is about the decisions the schema leaves to you — what to
put in `⟨l⟩`, how to size `Δ`, and what the identifiers can and cannot settle
once the partition heals. The figures quoted are measured against this
implementation.

## The shape of the problem

Owners `A`, `B`, `C` hold addresses on a ring; each key is owned by the `N`
nodes clockwise from it. A node fails silently, or a partition splits the
cluster, and for some interval two owners accept writes for the same range —
call that interval the **overlap**. When the cluster converges there are three
questions, and they are answered in three different places:

1. **Who wrote what?** Which owner produced a given record, and what is the
   complete set each of them produced during the overlap. **The layout answers
   this**, and answers it in the key space itself — no side channel, no
   secondary index.
2. **Does one version descend from the other, or are they concurrent?** The
   version vector answers this, and it answers it *with* these identifiers.
   Dominance is decided component by component, every component comparison is
   an intra-actor `Before`, and that comparison is exact rather than k-ordered
   ([Corollary 2](proof.md#47-corollary-2-intra-node-order-is-exact)). The
   identifiers are load-bearing here: the vector is only as correct as the
   per-actor order underneath it.
3. **Which of two concurrent versions survives?** Nothing in this library
   answers this, and nothing should. It is application policy — a merge
   function, a semantic rule, keeping siblings, or last-writer-wins with the
   identifier as an arbitrary but deterministic tie-break.

The line that matters is between 2 and 3, not between 1 and 2. `guid` is the
ordering primitive a version vector is built out of; it is not the resolution
policy. Collapsing the two gives you last-writer-wins keyed on ring position,
which is deterministic — every replica computes the same winner — but discards
one of the two writes without ever reporting that there were two.

## What the layout buys

`⟨l⟩` sits *inside* the timestamp: coarse time above it, fine time below. So
inside one epoch bucket of width `Δ` the sort order is grouped by allocator
into a contiguous **run**, and each run is exactly ordered
([Corollary 2](proof.md#47-corollary-2-intra-node-order-is-exact)).

```
   ⟨d⟩       ⟨E⟩          ⟨l⟩         ⟨xₗ⟩      ⟨s⟩
  drift   coarse time    owner     fine time  counter
```

One range scan of a bucket returns every owner's contribution to that bucket,
already partitioned by owner. Under a timestamp-primary schema — Snowflake,
UUIDv7 — the two owners' writes interleave and attribution needs a side
channel. That difference is the reason to choose this library for a ring.

The property is **per bucket, not per overlap**. An overlap of duration `T`
fragments into roughly `⌈T/Δ⌉+1` runs per owner, because each bucket boundary
restarts the grouping. Measured, two owners writing concurrently:

| `Δ` | overlap `T` | runs per owner |
|---|---|---|
| 17.2 s | 10 s | 2 |
| 17.2 s | 300 s | 19 |
| 274.9 s | 60 s | 1 |
| 274.9 s | 300 s | 2 |

Fragmentation is graceful — the runs are still exactly ordered and still
attributable — but a scan that expected one run per owner finds nineteen. Size
`Δ` against the overlap, see below.

## Choosing `⟨l⟩`

### Use the vnode token, not the physical node address

Both choices keep the two split-brain writers distinct, because a successor
serves the range from its own ring position rather than assuming the failed
node's identity. They differ in whether the sort order means anything.

A common tie-break is *lowest `⟨l⟩` wins*, evaluated with `Before`. **That is
the wrong comparison, and the error is not small.** `Before` orders `⟨l⟩` with
`<`, whose least element is `0`. A ring has no least element — it has one order
per cut point, and the one a key `k` means is

```
a <ₖ b  ⟺  (a − k) mod 2ᴺ  <  (b − k) mod 2ᴺ
```

`<` is the member of that family cut at the origin, so it answers for `k = 0`
whatever key you actually asked about. It is right only where the key's
preference list does not wrap the origin — a fraction `(M−N+1)/M` of the ring
for `M` distinct addresses and replication factor `N`:

| distinct `⟨l⟩` on the ring | `N` | `Before` agrees with the primary for |
|---|---|---|
| 3 | 3 | 33 % |
| 10 | 3 | 80 % |
| 256 | 3 | 99 % |
| 25 600 | 3 | 99.99 % |

With three physical nodes it misfires on two thirds of the key space:

```
key in seg A: pref=[B C A] primary=B lowest=A <- successor beats primary
key in seg B: pref=[C A B] primary=C lowest=A <- successor beats primary
key in seg C: pref=[A B C] primary=A lowest=A ok
```

`A` wins every conflict, including for keys it holds only as a second
successor.

**Compare with the ring order instead and the error is zero**, at every key and
for any placement of tokens — the primary for `k` is by construction the least
element of `<ₖ`:

```go
ord := guid.OrdRingG(key >> 32)   // the order cut at this key
slices.SortFunc(uids, ord.Compare)   // uids[0] belongs to the primary
```

There is no contradiction between the three segments above, and no topological
obstacle to getting them all right. `B <ₖ₁ C <ₖ₁ A` and `C <ₖ₂ A <ₖ₂ B` are
statements about two *different* relations, each a strict total order. Only
collapsing them onto one relation — which is what `Before` does — produces the
cycle `B < C < A < B` and with it the misfire.

So the table above is the error rate of the wrong comparator, not a limit to
engineer around. It matters only if you compare with `Before` anyway, which is
a reasonable choice when the tie-break has to be computable by a system that
cannot call into this library — a SQL `ORDER BY`, a KV store's native
collation. **That** is the trade: `Before` is sortable by anything and wrong on
`(N−1)/M` of the ring; `<ₖ` is exactly right and computable only in process,
see *What this costs* in [ring.go](../ring.go).

If the tie-break is only required to be *deterministic* — every replica
computing the same winner, with no claim that the winner is the owner — then
neither `M` nor the comparator matters, and `Before` is the cheaper choice.
Decide which of the two you mean before you rely on it.

Putting the **vnode token** in `⟨l⟩` rather than the host address remains the
right call either way, but for the reason this section opened with rather than
for the misfire rate: `<ₖ` orders by `⟨l⟩`, so `⟨l⟩` has to *be* ring position
for that order to mean ring adjacency. A host address gives you a well-defined
order over something that is not the topology, and `<ₖ` will sort it perfectly
and tell you nothing.

### Top-align a token wider than `⟨l⟩`

[`WithNodeID`](../clock.go#L165) masks to 58 bits, the width `X` gives `⟨l⟩`,
and [`makeG`](../global.go#L77) then keeps the **low** 32 of those. Feeding a
64-bit ring token straight in therefore discards the part that carries the
topology:

```
token 0000100000000000 -> G.Node()=00000000
token 4000000000000000 -> G.Node()=00000000
token 8000000000000000 -> G.Node()=00000000
token c000000000000000 -> G.Node()=00000000
!! 4 distinct ring addresses collapsed onto ⟨l⟩=00000000
```

Silent, and it destroys uniqueness and ring order together. Shift the token
down to the width of the field first:

```go
// 64-bit ring token, 96-bit identifiers
clock := guid.NewClock(
    guid.WithNodeID(token>>32),
    guid.WithDrift(guid.DriftOf(90*time.Second)),
)
```

```
token 4000000000000000 -> ⟨l⟩=40000000  ring order preserved=true
token 8000000000000000 -> ⟨l⟩=80000000  ring order preserved=true
```

For `guid.X` the field is 58 bits, so `token>>6` for a 64-bit ring, and there
is room to carry a token prefix in the high bits and a disambiguator in the
low ones if the two must be separated.

There is no birthday bound to plan against here. The bound quoted in
[short.md](short.md#where-it-breaks) applies to
[`WithNodeRandom`](../clock.go#L198); addresses claimed on a ring are distinct
by the same mechanism that stops two vnodes occupying one position.

### Several vnodes in one process

One `Chronos` per token. Clocks built on the default unix ticker share one
process-wide `⟨t,s⟩` sequence ([sequence.go:122](../sequence.go#L122)), so
values allocated across them stay unique and each token's own stream stays
exactly ordered. Verified at 50 000 allocations across three tokens: no
duplicates, intra-token order exact.

[`WithClock`](../clock.go#L228) is the exception — a custom ticker gets a
private sequence, since the library cannot know whether two custom generators
share a time domain.

## Sizing `Δ`

Read `Δ` as a **failover budget**: it has to cover the interval from silent
failure to the moment the cluster has converged on a new owner, because that is
the interval during which two owners write to one range. Pick the rung with
[`DriftOf`](../drift.go#L112) from your detector's worst case — lease expiry,
phi-accrual threshold, gossip convergence — not from your clock quality.

Two consequences follow from `Δ` bounding the grouping rather than the
ordering:

**The tie-break is `(bucket, owner)`, not `owner`.** Inside one bucket `⟨l⟩`
decides; across a boundary time decides and the later owner can win. So
*lowest wins* is stable only while `Δ` comfortably exceeds the overlap. At
`Drift17s` with a 300 s overlap the winner alternates 19 times.

This one is **not** fixed by the ring order. `<ₖ` rotates `⟨l⟩` and nothing
above it, and `⟨𝑬⟩` outranks `⟨l⟩` under either comparator — so a bucket
boundary resets the winner whichever one you use. Sizing `Δ` against the
overlap is the only remedy, exactly as above.

**A wider `Δ` costs ordering elsewhere.** Everything the cluster allocates
within `W = Δ + 2ε` is ordered by owner rather than by time, and a time range
scan over the same key space must widen by `W` on both sides. At the default
`Drift275s` a measured worst-case inversion spans 175 s. If the same key space
also serves "most recent" queries, that is the price.

## Identifiers inside a version vector

The intended composition: the version vector decides *concurrency*, `guid`
supplies the per-actor component. It is a good component for a reason specific
to uncoordinated deployment — a fresh process allocates above its own history
**without reading stored state**, because the value is clock-derived. That
removes the persist-and-recover-the-counter problem a plain integer has.

Comparison inside a version vector is always intra-actor, where the guarantee
is exact rather than k-ordered. The drift window never enters correctness:
verified at `Δ` = 73 min, 10 000 allocations at one `⟨l⟩`, exactly ordered.

Which type:

| role | type | `⟨l⟩` |
|---|---|---|
| version vector component, dot | [`guid.L`](../local.go#L48), 8 B | none — the actor is already the map key |
| journal or event storage key | [`guid.G`](../global.go#L55) 12 B, [`guid.X`](../extended.go#L112) 16 B | vnode token, top-aligned |

`L` is the right component: strictly ordered, no drift to reason about, and it
does not re-encode an actor the vector already records.
[`G.FromL`](../global.go#L307) promotes a dot to a scannable key when one needs
to become a row.

### The restart hazard

The `⟨t,s⟩` high-water mark lives in a process-global sequence that is zeroed
at start. Within a process a backwards clock step is harmless — the `max` does
not fire and the counter continues from its mark. **Across a restart it is
not**, because nothing carries the mark over:

```
same actor across restart, clock stepped back 5s:
  before=72.BB5DB39813..0  after=72.BB5DB14407..0  monotone=false
  same step back, no restart:                      monotone=true
```

Restart duration works in your favour: the exposure is the size of the
backwards step *minus* how long the process was down, so a 5–10 s restart
absorbs an ordinary correction. What it does not absorb is a step taken at
boot, which is exactly when the process is not running to hold a mark through
it — a node that drifted while cut off from NTP takes its correction then, and
its size is `ε`. A deployment that chose a high rung because `ε` is tens of
seconds cannot also assume steps are smaller than a restart.

The failure mode is silent. A component below the stored high-water makes a new
write look dominated, so it is dropped as stale rather than reported as an
error.

The guard is cheap — floor the ticker at the value the store already indexes
per actor:

```go
hw := storedHighWater(actor) + 1<<17 // one tick above, see below

clock := guid.NewClock(
    guid.WithNodeID(token>>32),
    guid.WithDrift(drift),
    guid.WithClock(func() uint64 {
        t := uint64(time.Now().UnixNano())
        if hw > t {
            return hw
        }
        return t
    }),
)
```

Advance by one tick. Flooring at the stored value exactly *reproduces* it — a
duplicate dot, which is worse than an inversion:

```
floored at the stored high-water:      before=…9813..0 after=…9813..0  monotone=false
floored at the high-water + one tick:  before=…9813..0 after=…9817..0  monotone=true
```

One tick is `2¹⁷` ns ≈ 131 µs, the resolution
[`Time`](../local.go#L89) reports at.

## What this does not give you

**Concurrency detection from a single comparison.** Two identifiers compared
against each other always yield a total order, so one `Before` can never report
*concurrent*. This is the reason they go *into* a version vector rather than
replacing one — not a reason to keep them out of it. The vector's dominance
test is what separates "B wrote after A" from "A and B wrote in a partition";
the identifiers are what each component of that test compares.

**Ownership authority — if you compare with `Before`.** *Lowest `⟨l⟩` wins*
selects an owner, not the primary, outside the fraction of the ring tabulated
above. Compared with `OrdRingG(key)` it *does* select the primary, at every
key, so sort position recovers ownership exactly. The caveat that remains is
narrower: the identifier records the owner that allocated it, which is the
owner the ring had *then*. If membership changed between the write and the
read, sort position answers for the old ring, and only a recorded intent
answers for the one you are recovering into.

**Real-time order across owners.** Inside `W = Δ + 2ε` the order you read back
is ring order, deliberately. A write made later can sort earlier. This is not a
limitation to engineer around — [§5 of proof.md](proof.md#5-what-is-not-claimed)
records that no scheme comparing two identifiers alone can recover real-time
order without coordination.

## Checklist

- [ ] `⟨l⟩` is the vnode token, not the host address, if the sort order is meant
      to track the ring
- [ ] the token is shifted to the top of `⟨l⟩` — 32 bits for `G`, 58 for `X`
- [ ] one `Chronos` per token, all on the default ticker or all seeded alike
- [ ] `Δ` chosen from the failure detector's worst case, and the same on every
      node of the keyspace
- [ ] the same `TimeOrder` on every node of the keyspace
- [ ] version vector components are `guid.L`; actors are distinct per writer
- [ ] the ticker is floored at the stored high-water plus one tick on startup
- [ ] conflict resolution is the version vector's, with the identifier as
      tie-break only
- [ ] the tie-break compares with `OrdRingG`/`OrdRingX` cut at the key,
      not with `Before` — or `Before` is used deliberately, knowing it answers
      for the origin and misfires on `(N−1)/M` of the ring
