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

| | size | node id | |
|---|---|---|---|
| `guid.L` | 8 B | — | unique within one allocator |
| `guid.G` | 12 B | 32 bit | ≈ 65 000 allocators before collisions matter |
| `guid.X` | 16 B | 58 bit | ≈ 5.4·10⁸, and it **is** an RFC 9562 UUID (v8) |

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
a ladder of eight rungs, `guid.Drift1ms` … `guid.Drift4398s`. Default
`guid.Drift275s` — 274.9 s — and the ladder runs from 1.05 ms to 73 min.
`guid.DriftOf(d)` picks the smallest rung that covers a tolerance `d`.

**Read `Δ` as a failover budget, not a clock-skew budget.** It has to cover the
interval between a silent failure and the moment the cluster has converged on a
new owner, because that is exactly the interval during which two allocators
write to the same range:

| rung | `Δ` | covers |
|---|---|---|
| `Drift1ms` – `Drift268ms` | 1.05 ms – 268 ms | synchronized clocks; the ordering class of Snowflake and UUIDv7 |
| `Drift2s` – `Drift17s` | 2.15 s – 17.2 s | consumer devices, lease expiry, fast failure detectors |
| `Drift275s` *(default)* – `Drift1099s` | 274.9 s – 1099 s | gossip / phi-accrual detection, slow cross-region hand-over |
| `Drift4398s` | 4398 s | human-in-the-loop failover |

Below a second the window `Δ + 2ε` is dominated by the clock skew `ε` rather
than by `Δ`, so the low rungs pay off only where the clocks are genuinely
disciplined.

It is *also* the clock disagreement the ordering tolerates, which is why one
knob serves both. And that range is not over-generous: the target is not a
managed cluster with datacenter NTP but uncoordinated nodes — hardware with no
RTC, devices behind firewalls that block NTP, VMs resuming from snapshot,
phones returning from airplane mode. Tens of seconds of disagreement is the
distribution there, not a pathology.

> If ⟨l⟩ is meant to carry topology, **assign it rather than randomize it**.
> The default [`WithNodeRandom`](../clock.go#L197) gives distinct allocators,
> but random identities sort arbitrarily, so adjacent ring positions land far
> apart. Derive ⟨l⟩ from ring position with
> [`WithNodeID`](../clock.go#L164) when sort order should follow the topology.

## The three results

### 1. Local identifiers (`guid.L`, 8 bytes) are perfectly sorted

Not "approximately". If one process allocates `a` then `b`, then `a < b`.
Always.

The reason is simpler than you would expect: the process keeps **one counter**,
and the identifier *is* that counter plus a constant. The counter only moves up
— allocating does `+1`, a clock tick does `max(counter, clock)`. A number that
only increases, incremented atomically, hands out increasing values. That is
the whole proof (Theorem 1, §3.5).

Notice what is missing: no assumption about the clock. NTP can step the clock
backwards and ordering still holds — the `max` simply does not fire and the
counter continues from its high-water mark. No assumption about rate either.
You cannot break this by allocating too fast.

> **This used to be false.** Older versions drew the timestamp and the counter
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
> always correctly ordered. Two identifiers allocated **closer than that** may
> be in either order.

where `ε` is your worst clock skew. Disorder exists, but it is confined to a
window, and you know the window.

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

> **The global stream is N perfectly sorted streams, merged sloppily.**

Every inversion is between nodes. None is within a node. If you shard by node,
each shard is exactly ordered.

## What this means operationally

| you want to | do this |
|---|---|
| a sortable key in a database | use `L` — 8 bytes, strictly ordered, see below |
| tell two overlapping writers apart | scan each node's run; they do not interleave |
| range scan by time | widen the range by `W` on both sides |
| fully sort the stream | buffer `k` entries — but see below |
| exact ordering | stay within one node, or use `L` |
| read a timestamp back | `Time()` is accurate to 131 µs, not a precise clock |

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

**Node id collisions — the real limit on cluster size.** In a `guid.G`, node
ids are 32 random bits. By the birthday bound you get roughly **65,000
allocators** before collision probability approaches ½. Two allocators sharing
a node id, in the same bucket, at the same fine time, with the same counter
value, produce *the same identifier*. Uniqueness is assumed, not proven — it is
the one thing here that is not.

If you need more allocators than that, either assign node ids explicitly with
[`WithNodeID`](../clock.go#L164) instead of randomly, or use `guid.X`, whose 58
random bits move the bound to about **5.4·10⁸** allocators. That is the reason
the wider type exists; being a UUID is the other one.

**Mixed drift settings — silent and nasty.** The drift code is the *top* field.
Identifiers made with different drift settings sort by their configuration
rather than their time. This is now hard to do by accident — drift lives on the
clock, not on each call — but two clocks built differently still feed one
keyspace if you let them. It shows up as a bimodally distributed key space, not
as an error.

**Underestimated clock skew.** If your real `ε` exceeds what you assumed, `W`
is wider than you budgeted. Nothing fails, nothing errors — you just get more
disorder than planned. Degradation here is graceful, which is also why it can
go unnoticed.

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
to reduce disorder are a smaller `Δ` or better clock sync — you cannot analyze
your way to a smaller number.
