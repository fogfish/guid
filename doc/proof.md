# Correctness of k-ordering in `guid`

This note proves two properties of the identifiers produced by this library.

> **(1)** Locally allocated values (`guid.L`, 64-bit, allocated by `NewL`) are
> **linearizable**: the
> order they induce is a valid sequential history of a strictly increasing
> generator — unconditionally, at any allocation rate and under any clock.
>
> **(2)** Globally allocated values (`guid.G`, 96-bit, allocated by `NewG`) are
> **k-ordered**:
>
> ```
>   𝑨[𝒊 − 𝒌] ≤ 𝑨[𝒊] ≤ 𝑨[𝒊 + 𝒌]   for all 𝒊 such that 𝒌 < 𝒊 ≤ 𝒏 − 𝒌
> ```
>
> where `𝑨` is the sequence of allocated values ordered by the real time of
> allocation, and `𝒌` is the number of allocations the whole cluster performs
> during one drift window `𝑾 = Δ + 2ε`.

Statement (2) is additionally machine-checked in Lean 4 — see
[proof.lean](proof.lean) and §6.

The proofs are stated against the actual bit layout produced by
[`makeG`](../global.go#L66) / [`makeL`](../local.go#L59) and the comparison operators
[`G.Before`](../global.go#L95) / [`L.Before`](../local.go#L75). §1 establishes that
layout; everything after that is arithmetic on a positional numeral system.

The layout is unchanged from v2. What v3 changed is the *storage* of it: `G` is
the 96-bit number in big-endian bytes and `L` is the 64-bit number itself, two
distinct types rather than one 128-bit struct carrying either. §1.6 records
what that buys the proofs.

---

## 1. The encoding

### 1.1 Notation

| symbol | meaning | width |
|---|---|---|
| `𝑫`   | drift parameter, `driftInBits` value, `𝑫 ∈ {18,…,25}` | — |
| `𝒅`   | drift code stored in the value, `𝒅 = 𝑫 − 18` | 3 bit |
| `𝒕`   | clock reading in nanoseconds | 64 bit |
| `𝒙`   | truncated clock, `𝒙 = ⌊𝒕 / 2¹⁷⌋` (unit ≈ 131 µs) | 47 bit |
| `𝑬`   | *epoch*, `𝑬 = ⌊𝒙 / 2^𝑫⌋ = ⌊𝒕 / 2^(17+𝑫)⌋` | 47−𝑫 bit |
| `𝒙ₗ`  | low clock bits, `𝒙ₗ = 𝒙 mod 2^𝑫` | 𝑫 bit |
| `𝒍`   | node (allocator) identity | 32 bit |
| `𝒔`   | per-process sequence, `𝒔 = 𝒏 mod 2¹⁴` for the `𝒏`-th call | 14 bit |
| `Δ`   | **drift window** `Δ = 2^(17+𝑫)` nanoseconds | — |
| `⟦𝒖⟧` | numeric value of a k-ordered value as an integer; for `G` the 96-bit big-endian number its bytes hold, for `L` the 64-bit number itself | — |
| `𝒖.hi`, `𝒖.lo` | the words of a `G`, `⟦𝒖⟧ = 𝒖.hi·2⁶⁴ + 𝒖.lo`, see [`G.words`](../global.go#L76) | 32, 64 bit |

`Δ` is the quantity the user configures, on the clock and not per allocation,
with `WithDrift`. [`driftInBits`](../common.go#L51) maps a requested tolerance
to the smallest `𝑫` whose window covers it:

| `𝑫` | 18 | 19 | 20 | 21 *(default)* | 22 | 23 | 24 | 25 |
|---|---|---|---|---|---|---|---|---|
| `Δ` | 34.4 s | 68.7 s | 137.4 s | **274.9 s** | 549.8 s | 1099.5 s | 2199.0 s | 4398.0 s |

`𝑫 = 18` is the floor, and it is geometry rather than policy: `𝒃 = 𝑫 − 18` is
the number of `⟨𝒍⟩` bits that fall above the `hi`/`lo` boundary, so a smaller
`𝑫` would spill `⟨𝑬⟩` across the word boundary — a layout neither `splitT` nor
`splitNode` expresses. The ladder is 3 bits wide, so 8 rungs exhaust it.

### 1.2 Proposition 1 (global layout)

For `𝑫 ∈ {18,…,25}`, `𝒍 < 2³²`, `𝒔 < 2¹⁴`:

```
  ⟦makeG(𝒍, 𝑫, 𝒕, 𝒔)⟧ = 𝒅·2⁹³ + 𝑬·2^(46+𝑫) + 𝒍·2^(14+𝑫) + 𝒙ₗ·2¹⁴ + 𝒔
```

i.e. the 96-bit word is the concatenation

```
     3      47 − 𝑫          32           𝑫        14
   |---|--------------|------------|----------|--------|
    ⟨𝒅⟩      ⟨𝑬⟩          ⟨𝒍⟩        ⟨𝒙ₗ⟩      ⟨𝒔⟩
```

*Proof.* Write `𝒂 = 64 − 14 − 𝑫` and `𝒃 = 32 − 𝒂 = 𝑫 − 18`, as in
[`splitT`](../common.go#L73) / [`splitNode`](../common.go#L95).

* `splitT` computes `lo = (𝒙 ≪ (𝒂+14)) ≫ 𝒂`. Since `𝒂 + 14 = 64 − 𝑫`, only the
  low `𝑫` bits of `𝒙` survive the left shift inside a 64-bit register, and the
  subsequent right shift by `𝒂` places them at positions `14 … 13+𝑫` of `Lo`.
  Hence `lo_t = 𝒙ₗ·2¹⁴`.
* `splitT` computes `hi = (𝒙 ≫ 𝑫) ≪ 𝒃 = 𝑬·2^𝒃`, occupying bits `𝒃 … 28` of `hi`
  (`𝑬` has `47 − 𝑫` bits), and `dd = (𝑫−18) ≪ 29 = 𝒅·2²⁹`.
* `splitNode` computes `lo = 𝒍 ≪ (𝑫+14)`, of which the low `𝒂 = 50 − 𝑫` bits of
  `𝒍` survive in `lo` at positions `𝑫+14 … 63`, and `hi = 𝒍 ≫ (32−𝒃)`, the top
  `𝒃` bits of `𝒍` at positions `0 … 𝒃−1` of `hi`.

`makeG` ORs these into `hi = hi_t | hi_l`, `lo = lo_l | lo_t | 𝒔`, then `joinG`
writes the pair out big-endian. The occupied ranges are pairwise disjoint and
contiguous, and the node halves are adjacent across the `hi`/`lo` boundary
(`𝒃` bits ending at `hi` bit 0, `𝒂` bits starting at `lo` bit 63), so the OR is
an addition and the claimed positional form follows. Widths check out:
`3 + (47−𝑫) + 32 + 𝑫 + 14 = 96`. ∎

> This proposition was also verified differentially against the implementation
> for every `𝑫 ∈ {18,…,25}` over 160 000 random `(𝒍, 𝒕, 𝒔)` triples: the
> `big.Int` model and `⟦makeG(...)⟧` agree bit for bit.

### 1.3 Proposition 2 (local layout)

```
  ⟦makeL(𝑫, 𝒕, 𝒔)⟧ = 𝒅·2⁶¹ + 𝒙·2¹⁴ + 𝒔
```

*Proof.* Immediate from [`makeL`](../local.go#L59): `d = (𝑫−18) ≪ 61`,
`x = 𝒕 ≫ 17 ≪ 14 = 𝒙·2¹⁴`, `seq = 𝒔 < 2¹⁴`; the three ranges are disjoint and
contiguous, `3 + 47 + 14 = 64`. ∎

Note that the local value keeps the *whole* `𝒙` above `𝒔` — it is not split
around a node field, because there is no node field.

### 1.4 Lemma 1 (order homomorphism)

`Before(a, b) ⟺ ⟦a⟧ < ⟦b⟧`.

*Proof.* For `L`, [`Before`](../local.go#L75) is `uint64` comparison and `⟦·⟧` is
the identity, so the claim is trivial. For `G`,
[`Before`](../global.go#L95) is `a.hi < b.hi ∨ (a.hi = b.hi ∧ a.lo < b.lo)`, which
is exactly the lexicographic comparison of the base-2⁶⁴ representation
`⟦·⟧ = hi·2⁶⁴ + lo` of a non-negative integer. ∎

Because `G` stores `⟦·⟧` big-endian, the same lemma holds for `bytes.Compare`
over the raw 12 bytes: byte-lexicographic order on a fixed-width big-endian
numeral *is* numeric order. The library therefore has two agreeing comparators,
and an external index that sorts the stored bytes — a B-tree, a sorted file, a
key-value store — orders the identifiers correctly without decoding them.

### 1.5 Lemma 2 (positional comparison)

Let `𝑭 = (𝒇₁,…,𝒇ₘ)` be fields of widths `𝒘₁,…,𝒘ₘ`, i.e. `𝒇ⱼ < 2^𝒘ⱼ`, packed as
`⟦𝑭⟧ = Σⱼ 𝒇ⱼ·2^(𝒘ⱼ₊₁+⋯+𝒘ₘ)`. Then `⟦𝑭⟧ < ⟦𝑮⟧` iff `𝑭 <ₗₑₓ 𝑮`.

*Proof.* Standard positional-numeral argument. The key step, used repeatedly
below, is: if `𝒑 < 𝒒` and `𝒓 < 𝒃` then `𝒑·𝒃 + 𝒓 < 𝒒·𝒃`, because
`𝒑·𝒃 + 𝒓 < 𝒑·𝒃 + 𝒃 = (𝒑+1)·𝒃 ≤ 𝒒·𝒃`. Induction on `𝒎` gives the claim. ∎

By Propositions 1–2 and Lemmas 1–2:

* global values with equal drift code compare **lexicographically on
  `(𝑬, 𝒍, 𝒙ₗ, 𝒔)`**;
* local values with equal drift code compare **lexicographically on `(𝒙, 𝒔)`**.

### 1.6 The v3 representation

v2 stored both shapes in one `struct{ Hi, Lo uint64 }`, 128 bits of which 96
were ever used, and distinguished them by `Hi = 0`. Three obligations of this
note came from that choice, and v3 discharges all three by construction:

* **Slack bits.** 32 of `K`'s 128 bits were never written by `makeG` and never
  read by `Before`, yet they were part of the value, so equality and map-key
  identity depended on bits the schema does not define. `L` and `G` are exactly
  as wide as the schema, so every bit of a value is a field of it and `==`
  agrees with `Before`-equality.
* **Mixed comparison.** `Before` over a local and a global value was
  meaningless but well-typed, and put every local value ahead of every global
  one. `L.Before` takes an `L` and `G.Before` takes a `G`, so the comparison no
  longer type-checks; §5 records what replaced the warning.
* **Discrimination.** `Hi = 0` doubled as "this is a local value", which is a
  property of the *value*, not of the type. A global value with `𝑫 = 18` and
  `𝑬 = 0` also has `hi = 0`; §7 had to argue that `driftInBits` never returns
  18 to rule it out. The types carry the distinction now, so the argument is
  no longer load-bearing — and `𝑫 = 18` was re-enabled once it was not, which
  is where the `Δ = 34.4 s` rung of §1.1 comes from.

---

## 2. Assumptions

The theorems below are conditional; every hypothesis is named so that §7 can
discuss when it fails.

* **(A1) Fixed drift.** All compared values were produced with the same `𝑫`.
  (The drift code is the most significant field, so values of different `𝑫`
  are segregated rather than interleaved.)
* **(A2) Coupled allocation.** The pair `⟨𝒕,𝒔⟩` is drawn from the allocator of
  [sequence.go](../sequence.go), reproduced as Algorithm 1 in §3.2. This is not an
  assumption about the environment but a description of the implementation, and
  §3.3 discharges it into a property.
* **(A5) Bounded skew.** Every node's clock satisfies `|C_𝒍(τ) − τ| ≤ ε` for
  all real times `τ`, for a common bound `ε`.
* **(A6) Rate bound.** At most `𝒌` allocations occur cluster-wide in any
  half-open real-time window of length `𝑾 = Δ + 2ε`.
* **(A7) Distinct allocators.** Distinct processes use distinct `𝒍`.

Assumptions (A5)–(A7) concern the cluster and are needed only for Part II.
Part I needs (A1) and (A2) alone. In particular it needs **no** assumption
about clock monotonicity and **no** bound on the allocation rate; earlier
versions of this library required both, and §7 records why.

## 3. Part I — local values are linearizable

### 3.1 The object

Model `L` as a concurrent object `Gen` with a single operation `alloc()`.
Its **sequential specification** is: *`alloc()` returns a value strictly
greater than every value returned by an earlier `alloc()`*.

Recall the definition. A concurrent history `H` is **linearizable** with
respect to a sequential specification `Σ` iff there exists a total order `S`
on the completed operations of `H` such that

1. `S` extends the real-time precedence order `<ᵣₜ` of `H`
   (`op_p <ᵣₜ op_q` when `op_p` responds before `op_q` is invoked), and
2. the sequence of operations in the order `S` belongs to `Σ`.

### 3.2 The allocator

By Proposition 2 the order of a local value is decided by `(𝒙, 𝒔)` compared
lexicographically, which by Lemma 2 is the order of the single number

```
  𝑽 = 𝒙·2¹⁴ + 𝒔 .
```

`𝑽` is the object the implementation actually maintains — one machine word,
shared by all allocators of a time domain:

> **Algorithm 1** (`sequence.next`, [sequence.go](../sequence.go)). On allocation
> with clock reading `𝒕`, let `𝒃 = ⌊𝒕/2¹⁷⌋·2¹⁴`.
>
> 1. *advance.* If `𝒃` exceeds the last observed tick, one allocator wins a
>    `CAS` on the tick word and raises `𝑽` to `max(𝑽, 𝒃)`.
> 2. *allocate.* Return `𝑽 ← 𝑽 + 1`, one atomic add.
>
> Return `𝒙 = ⌊𝑽/2¹⁴⌋`, `𝒔 = 𝑽 mod 2¹⁴`.

### 3.3 Lemma 3 (the allocator is monotone)

*Order the completed `alloc()` operations by their atomic add in step 2 — a
total order, since the adds are atomic operations on one word. Let `𝑽₁, 𝑽₂, …`
be the values they return, in that order. Then `𝑽ᵢ < 𝑽ⱼ` for `𝒊 < 𝒋`, and every
`𝑽ᵢ` is distinct.*

*Proof.* Only two kinds of operation write the word: the add of step 2, which
increases it by 1, and the raise of step 1, which replaces `𝑽` by `max(𝑽, 𝒃) ≥ 𝑽`.
So the word is non-decreasing over the whole execution, and *strictly* increases
at each add. An add returns the post-increment value, which strictly exceeds the
value of the word immediately before it, hence strictly exceeds every value the
word has ever held — in particular every previously returned `𝑽`. ∎

Note what the proof does not use: the clock never appears. Step 1 can raise `𝑽`
by any amount or not at all; `𝒃` may be stale, may repeat, may jump backwards.
Monotonicity is a property of the two write forms alone.

### 3.4 Lemma 4 (the encoding transports it)

*If `𝑽 < 𝑽′` then the local values built from them satisfy `⟦𝒗⟧ < ⟦𝒗′⟧`.*

*Proof.* By Proposition 2, the value built from `𝑽` is

```
  ⟦𝒗⟧ = 𝒅·2⁶¹ + ⌊𝑽/2¹⁴⌋·2¹⁴ + (𝑽 mod 2¹⁴) = 𝒅·2⁶¹ + 𝑽 ,
```

by the division identity `⌊𝑽/2¹⁴⌋·2¹⁴ + (𝑽 mod 2¹⁴) = 𝑽`. The drift codes are
equal by (A1), so comparing values is comparing `𝑽`. ∎

This is `Guid.packL_split` and `Guid.packL_lt_of_lt` in [proof.lean](proof.lean).

### 3.5 Theorem 1 (linearizability)

*Under (A1) and (A2), every history of `NewL` is linearizable with respect to
the strictly-increasing-generator specification, with the atomic add of step 2
as the linearization point.*

*Proof.* The add of step 2 lies between the invocation and the response of its
own operation, so ordering the operations by their adds gives a total order `S`
that extends `<ᵣₜ`: if `op_p` responds before `op_q` is invoked, then `op_p`'s
add precedes `op_q`'s. By Lemma 3 the returned `𝑽` are strictly increasing along
`S` and pairwise distinct; by Lemma 4 so are the returned values. Hence the
sequential history induced by `S` satisfies the specification, and `S` is a
linearization of `H`. ∎

Three remarks.

* **No rate bound, no clock assumption.** Theorem 1 holds at any allocation
  rate and for any clock — including one stepped backwards by NTP, where step 1
  simply never fires and `𝑽` continues from its high water mark. The cost is
  that `𝒙` then reports that high water mark rather than the current reading;
  ordering is exact, the timestamp is an over-estimate.
* **Sortedness.** Indexing local values by their linearization order yields a
  strictly increasing sequence: `𝒌`-ordered with `𝒌 = 1`. Local allocation is
  the degenerate, perfectly sorted case of the global construction, obtained by
  deleting the `⟨𝒍⟩` field.
* **Run-ahead.** If an allocator issues more than `2¹⁴` values while the clock
  stands still, `𝑽` carries into ticks the clock has not reached: `𝒙` runs ahead
  of real time by `2¹⁷ ns` per `2¹⁴` values over budget, i.e. **8 ns per value**.
  Sustained headroom before any run-ahead accumulates is `2¹⁴` per `2¹⁷ ns`, or
  `1.25·10⁸` values per second per process. Run-ahead is self-correcting: it
  decays as soon as the burst ends. §3.6 gives the rates.

### 3.6 Run-ahead under load

Theorem 1 costs nothing in *order*. It costs something in *accuracy*: an
allocator that outruns its clock reports a `⟨𝒕⟩` that has not happened yet.
This section quantifies how fast that gap opens, how fast it closes, and at
what load either becomes observable.

#### The two flows

By Lemma 4, `𝑽` is the timestamp denominated in units of

```
  𝒒 = 2¹⁷/2¹⁴ = 2³ = 8 ns ,
```

so both writers of the word move it in the same currency: *allocate* adds `1`,
which is `𝒒` of clock-face, and *advance* pins the word to the clock. Define the
**divergence** of an allocator whose clock reads `C(τ)` at real time `τ`:

```
  δ(τ) = 𝒒·𝑽(τ) − C(τ)   ≥ 0 .
```

It is non-negative because step 1 only ever raises `𝑽` to the clock, never
lowers it — run-ahead is one-sided, which matters in §4.

#### The equation

Let `ρ(τ)` be the instantaneous allocation rate of the process. In the fluid
limit (many allocations per tick, so the `+1` steps are a flow), allocation
contributes `𝒒·ρ` of clock-face per second while real time contributes `10⁹` ns
per second, and the advance step acts as a reflecting barrier at `δ = 0`:

```
  dδ/dτ  =  𝒒·ρ(τ) − 10⁹        while δ > 0
  δ      ≥  0                    (advance pins it when the clock catches up)
```

Writing the break-even rate

```
  ρ* = 10⁹/𝒒 = 1.25·10⁸  allocations per second,
```

the equation is just

```
  dδ/dτ = 𝒒·(ρ − ρ*) ,        δ ≥ 0 .
```

This is a fluid queue, and recognizing it as one is the shortest route to every
statement below: each allocation is an arrival demanding `𝒒` of time-budget,
the clock is a server draining budget at rate 1, and `δ` is the backlog. The
library's run-ahead is a leaky bucket whose leak rate is the passage of time.

#### Gain

For a constant `ρ > ρ*` the backlog grows linearly with slope

```
  g(ρ) = 𝒒·(ρ − ρ*) = 8ρ − 10⁹     [ns of divergence per second of real time]
  g(ρ) = 0                          for ρ ≤ ρ*
```

```
   g(ρ)
   s/s |
   3.0 |              :                                      ***
   2.7 |              :                                  ****
   2.3 |              :                             *****
   2.0 |              :                        *****
   1.7 |              :                    ****
   1.3 |              :               *****
   1.0 |              :           ****
   0.7 |              :      *****
   0.3 |              :  ****
   0.0 |*****************
       +--------------------------------------------------------
        0            ρ*            2ρ*          3ρ*          4ρ*
```

The hinge is the whole picture: below `ρ*` the gain is not small, it is
**exactly zero** — the advance step re-anchors `𝑽` to the clock on every tick
and no history accumulates. Above it the divergence grows without bound for as
long as the load lasts.

A burst of `𝑵` allocations delivered at rate `ρ > ρ*` lasts `𝑵/ρ` and therefore
peaks at

```
  δ_peak = 𝒒·𝑵·(1 − ρ*/ρ)   ⟶   𝒒·𝑵 = 8𝑵 ns   as ρ → ∞ .
```

An instantaneous burst of `𝑵` identifiers puts the clock face `8𝑵` ns ahead.
Sanity check: `𝑵 = 2¹⁴` gives `8·16384 = 2¹⁷ ns`, exactly one tick — the
"`2¹⁷ ns` per `2¹⁴` values over budget" of §3.5.

#### Cool-down

For `ρ < ρ*` the same equation runs backwards, with slope

```
  c(ρ) = 𝒒·(ρ* − ρ) = 10⁹ − 8ρ     [ns recovered per second of real time]
```

so that `δ(τ) = max(0, δ_peak − c(ρ)·τ)` and the gap closes after

```
                δ_peak             δ_peak         1
  τ_cool  =  ------------  =  ( ---------- ) · ---------- .
              10⁹ − 8ρ             10⁹          1 − ρ/ρ*
```

The first factor is the divergence read as a duration; the second is a
**stretch factor** set by the background load. At idle the allocator recovers
one nanosecond of divergence per nanosecond of real time, so `τ_cool` in
seconds is numerically `δ_peak` in nanoseconds over `10⁹` — the divergence pays
itself back in its own units.

```
   δ
  ms  |
 11.0 |
 10.0 |                 *o
  9.0 |               ** **oo
  8.0 |             **     **oooo
  7.0 |           **         *   ooo
  6.0 |          *            **    oooo
  5.0 |        **               **      ooo
  4.0 |      **                   *        ooo
  3.0 |     *                      **         oooo
  2.0 |   **                         **           ooo
  1.0 | **                             *             ooo
  0.0 |*                                ***************************
      +------------------------------------------------------------
       0               10               20               30     τ, ms

       burst at 2ρ* for 10 ms, then:   * idle (ρ = 0)   o ρ = ρ*/2
```

Both curves share the rise — gain depends only on the burst — and differ only
in the drain. The stretch factor is the practical content:

| background `ρ` | `1/(1 − ρ/ρ*)` | 10 ms of divergence clears in |
|---|---|---|
| 0 (idle) | 1 | 10 ms |
| `ρ*/2` | 2 | 20 ms |
| `0.9 ρ*` | 10 | 100 ms |
| `0.99 ρ*` | 100 | 1 s |
| `≥ ρ*` | ∞ | never |

#### Without the fluid approximation

The continuous form above is a convenience; the exact statement needs no limit.
Let `A(s,τ)` be the number of allocations in `(s,τ]`. Since `𝑽` advances by `𝒒`
per allocation and is reflected upward to the clock, the backlog obeys the
Lindley recursion, whose closed form is the supremum over all past windows:

```
  δ(τ) = max ( 0 ,  sup  [ 𝒒·A(s,τ) − (τ − s)·10⁹ ] ) .
                   s ≤ τ
```

Two consequences follow directly, and neither mentions a peak rate:

* **Boundedness is a statement about averages.** `δ` stays bounded iff some
  window-length-normalized excess is bounded, i.e. iff the *time-averaged*
  allocation rate stays below `ρ*`. A process may exceed `ρ*` arbitrarily often
  without accumulating anything, provided it is under `ρ*` on average. The peak
  rate sets the slope of an excursion; the mean rate decides whether the
  excursions return.
* **Sub-tick run-ahead is not observable.** A tick is `2¹⁷ ns` and holds `2¹⁴`
  sequence slots, so a process under budget ends any tick with
  `δ < 2¹⁴·𝒒 = 2¹⁷ ns` — one tick, which is precisely the resolution of
  `Time(uid)`. Below `ρ*` the divergence is therefore always smaller than the
  quantity it perturbs, and no reader can detect it.

#### What it costs, in numbers

| sustained `ρ` | `g(ρ)` | divergence after 1 s | after 1 min |
|---|---|---|---|
| 10⁶ /s | 0 | 0 | 0 |
| 10⁷ /s | 0 | 0 | 0 |
| 10⁸ /s | 0 | 0 | 0 |
| `ρ*` = 1.25·10⁸ /s | 0 | 0 | 0 |
| 2·10⁸ /s | 0.6 s/s | 600 ms | 36 s |
| 10⁹ /s | 7 s/s | 7 s | 7 min |

The first four rows are the design's actual answer: `𝒒` is about the cost of
the `lock xadd` in [`sequence.next`](../sequence.go) itself, so a process cannot
reach `ρ*` while doing anything with the identifiers it allocates. Run-ahead is
reachable only by a loop that allocates and discards.

#### Consequence for Part II

Run-ahead is indistinguishable from a fast clock, so it enters §4 as skew. It
is one-sided, which makes the accounting asymmetric: the values an allocator
produces are built from the effective clock `C(τ) + δ(τ)`, so under (A5)

```
  𝒄ⱼ ≥ τⱼ − ε                    (run-ahead only pushes readings up)
  𝒄ᵢ ≤ τᵢ + ε + δ_max
```

and re-running the proof of Lemma 7 with these bounds gives the inversion
window

```
  𝑾 = Δ + 2ε + δ_max
```

rather than `Δ + 2ε`. A cluster whose allocators stay under `ρ*` has
`δ_max < 2¹⁷ ns = 131 µs`, which is `2¹⁷/2³⁵ = 2⁻¹⁸ ≈ 3.8·10⁻⁶` of the
smallest available `Δ`;
the term is real but never the one that matters. A cluster that sustains
`ρ > ρ*` has an unbounded `δ_max` and therefore no `𝒌`-ordering guarantee at
all — which is the load bound (A6) seen from the other side.

## 4. Part II — global values are k-ordered

### 4.1 Setting

Fix `𝑫` (A1) and let `Δ = 2^(17+𝑫)`. Let the cluster perform `𝒏` allocations;
index them `1,…,𝒏` by the real time of allocation, `τ₁ ≤ τ₂ ≤ ⋯ ≤ τₙ` (ties
broken arbitrarily), and let `𝑨[𝒊]` be the value returned by the `𝒊`-th one,
allocated on node `𝒍ᵢ` from clock reading `𝒄ᵢ = C_{𝒍ᵢ}(τᵢ)`.

Throughout, `𝑨[𝒊] ≤ 𝑨[𝒋]` abbreviates `⟦𝑨[𝒊]⟧ ≤ ⟦𝑨[𝒋]⟧`, which by Lemma 1 is
`¬After(𝑨[𝒊], 𝑨[𝒋])`.

### 4.2 Lemma 5 (epoch dominance)

If `𝑬(𝒖) < 𝑬(𝒗)` and both have drift code `𝒅`, then `⟦𝒖⟧ < ⟦𝒗⟧`.

*Proof.* By Proposition 1, `⟦𝒖⟧ = 𝒅·2⁹³ + 𝑬(𝒖)·2^(46+𝑫) + 𝒓(𝒖)` with
`𝒓(𝒖) = 𝒍·2^(14+𝑫) + 𝒙ₗ·2¹⁴ + 𝒔`. The residue is bounded:

```
  𝒓(𝒖) ≤ (2³²−1)·2^(14+𝑫) + (2^𝑫−1)·2¹⁴ + (2¹⁴−1) = 2^(46+𝑫) − 1 .
```

Hence, using `𝑬(𝒖) + 1 ≤ 𝑬(𝒗)`,

```
  ⟦𝒖⟧ ≤ 𝒅·2⁹³ + 𝑬(𝒖)·2^(46+𝑫) + 2^(46+𝑫) − 1
      <  𝒅·2⁹³ + (𝑬(𝒖)+1)·2^(46+𝑫)  ≤  𝒅·2⁹³ + 𝑬(𝒗)·2^(46+𝑫)  ≤  ⟦𝒗⟧ . ∎
```

This is *the* structural fact of the schema: the node identity outranks the low
`𝑫` bits of the clock, but nothing outranks the epoch. Clock disagreement can
permute values only *inside* an epoch.

### 4.3 Lemma 6 (epoch is a coarse clock)

`𝑬(𝑨[𝒊]) = ⌊𝒄ᵢ / Δ⌋`.

*Proof.* `𝑬 = ⌊𝒙/2^𝑫⌋ = ⌊⌊𝒄ᵢ/2¹⁷⌋/2^𝑫⌋ = ⌊𝒄ᵢ/2^(17+𝑫)⌋ = ⌊𝒄ᵢ/Δ⌋`, by the
nested floor identity `⌊⌊𝒂/𝒃⌋/𝒄⌋ = ⌊𝒂/(𝒃𝒄)⌋` for positive integers `𝒃, 𝒄`. ∎

### 4.4 Lemma 7 (inversion window) — the core estimate

*Assume (A1), (A5). If `𝒊 < 𝒋` and `𝑨[𝒊] > 𝑨[𝒋]` (an inversion), then*

```
  τⱼ − τᵢ  <  𝑾  :=  Δ + 2ε .
```

*Proof.* We prove the contrapositive: assume `τⱼ ≥ τᵢ + Δ + 2ε`. By (A5),
`𝒄ⱼ ≥ τⱼ − ε` and `𝒄ᵢ ≤ τᵢ + ε`, so

```
  𝒄ⱼ  ≥  τⱼ − ε  ≥  τᵢ + Δ + ε  ≥  𝒄ᵢ + Δ .
```

Dividing by `Δ > 0` and using `⌊(𝒎+Δ)/Δ⌋ = ⌊𝒎/Δ⌋ + 1`:

```
  ⌊𝒄ⱼ/Δ⌋  ≥  ⌊(𝒄ᵢ+Δ)/Δ⌋  =  ⌊𝒄ᵢ/Δ⌋ + 1  >  ⌊𝒄ᵢ/Δ⌋ ,
```

so by Lemma 6 `𝑬(𝑨[𝒊]) < 𝑬(𝑨[𝒋])`, and by Lemma 5 `𝑨[𝒊] < 𝑨[𝒋]`: not an
inversion. ∎

Note what the lemma does *not* need: no bound on the number of nodes, no
assumption about node identities, no synchronization. The only inputs are the
skew bound and the position of the epoch field.

### 4.5 Theorem 2 (k-orderedness)

*Assume (A1), (A5), (A6) with `𝒌` the maximum number of allocations in any
window of length `𝑾 = Δ + 2ε`. Then for all `𝒊 < 𝒋` with `𝒋 − 𝒊 ≥ 𝒌`,*

```
  𝑨[𝒊] ≤ 𝑨[𝒋] .
```

*In particular*

```
  𝑨[𝒊 − 𝒌] ≤ 𝑨[𝒊] ≤ 𝑨[𝒊 + 𝒌]   for all 𝒊 with 𝒌 < 𝒊 ≤ 𝒏 − 𝒌,
```

*i.e. the sequence of globally allocated values is `𝒌`-ordered.*

*Proof.* Let `𝒊 < 𝒋` with `𝒋 − 𝒊 ≥ 𝒌`. The allocations with indices
`𝒊, 𝒊+1, …, 𝒋` number `𝒋 − 𝒊 + 1 ≥ 𝒌 + 1`, and their times all lie in
`[τᵢ, τⱼ]`. If we had `τⱼ − τᵢ < 𝑾`, all of them would lie in the half-open
window `[τᵢ, τᵢ + 𝑾)`, contradicting (A6), which allows at most `𝒌`. Hence
`τⱼ − τᵢ ≥ 𝑾`, and by the contrapositive of Lemma 7 there is no inversion:
`𝑨[𝒊] ≤ 𝑨[𝒋]`.

For the displayed form, take `𝒋 = 𝒊` and `𝒊 = 𝒊 − 𝒌` (legal since `𝒌 < 𝒊`) for
the left inequality, and `𝒊, 𝒋 = 𝒊 + 𝒌` (legal since `𝒊 + 𝒌 ≤ 𝒏`) for the
right one. ∎

### 4.6 Corollary 1 (bounded displacement)

Let `rank(𝒊)` be the position of `𝑨[𝒊]` in the sorted permutation of `𝑨`. Then
`|rank(𝒊) − 𝒊| < 𝒌`.

*Proof.* By Theorem 2, every `𝒋 > 𝒊` with `𝑨[𝒋] < 𝑨[𝒊]` satisfies `𝒋 − 𝒊 < 𝒌`,
so fewer than `𝒌` elements to the right of `𝒊` are smaller; symmetrically fewer
than `𝒌` elements to the left are larger. `rank(𝒊)` differs from `𝒊` by exactly
the difference of these two counts. ∎

This is the form that matters in practice: a `𝒌`-ordered stream can be fully
sorted by a sliding window (or heap) of size `𝒌`, and a range scan over a
`𝒌`-ordered index must widen its bounds by `𝒌` entries — equivalently, by `𝑾`
of wall-clock time.

### 4.7 Corollary 2 (intra-node order is exact)

*Assume additionally (A2), (A7). If `𝒊 < 𝒋` and `𝒍ᵢ = 𝒍ⱼ`, then
`𝑨[𝒊] < 𝑨[𝒋]` regardless of `𝒋 − 𝒊`.*

*Proof.* With equal drift code and equal `𝒍`, lexicographic comparison on
`(𝑬, 𝒍, 𝒙ₗ, 𝒔)` reduces to lexicographic comparison on `(𝑬, 𝒙ₗ, 𝒔)`. Since
`𝒙 = 𝑬·2^𝑫 + 𝒙ₗ`, comparing `(𝑬, 𝒙ₗ)` lexicographically is the same as
comparing `𝒙`; so the order reduces to the lexicographic order on `(𝒙, 𝒔)`,
which is precisely the local order of §3 — that is, to the order of `𝑽`.
Lemmas 3 and 4 then apply verbatim. ∎

So the global sequence is an `𝑵`-way **merge of `𝑵` individually sorted
streams**, and every inversion is a cross-node inversion. Theorem 2 bounds how
far the merge can be off; Corollary 2 says the disorder is entirely inter-node.

Operationally this corollary, not Theorem 2, is the one the library is built
around. Combined with Lemma 5 it says that inside one epoch the key space is
*partitioned by allocator*: each node's values form a contiguous run, and each
run is exactly sorted. That is the property a leader-follower hand-over needs.
When a leader fails silently there is an interval in which two nodes write to
the same range; under a schema that ranks the whole timestamp above node
identity their writes interleave and attribution requires a side channel, while
here they occupy disjoint ranges that can be scanned, bounded and reconciled.
`Δ` is the width of that interval, which is why §1.1 calls it a configured
tolerance and the README reads it as a failover budget — they are the same
quantity seen from the two ends. Theorem 2 is then best read as the *price*:
the bound on how far the merge can be off, paid in exchange for the partition.

### 4.8 Sharpness

`𝑾 = Δ + 2ε` cannot be replaced by anything smaller. Take two nodes with
`𝒍₁ > 𝒍₂`, node 1 running `+ε` fast and node 2 running `−ε` slow. Let node 1
allocate at real time `τ₁` with `𝒄₁ = τ₁ + ε = 𝒎Δ` (bottom of epoch `𝒎`) and
node 2 allocate at real time `τ₂` with `𝒄₂ = τ₂ − ε = (𝒎+1)Δ − 1` (top of the
same epoch). Both values are in epoch `𝒎`, so the node field decides and
`𝑨[1] > 𝑨[2]`, an inversion, while

```
  τ₂ − τ₁ = (𝒄₂ + ε) − (𝒄₁ − ε) = Δ − 1 + 2ε = 𝑾 − 1 .
```

A Monte-Carlo run of the real implementation (4 nodes, skews
`{0, +60 s, −60 s, +20 s}`, `𝑫 = 21`, 2·10⁵ allocations over a 4000 s span)
produces a largest inversion gap of **394 862 051 854 ns** against the
predicted bound `𝑾 = 394 877 906 944 ns` — within 0.004 % of the bound, as the
construction above predicts.

### 4.9 The value of `𝒌`

If the cluster allocates at a peak aggregate rate of `ρ` identifiers per
second, then `𝒌 = ⌈ρ·𝑾⌉ = ⌈ρ·(Δ + 2ε)⌉`. With `ε = 1 s`:

| `ρ` \ `𝑫` | 18 (`Δ`=34.4 s, floor) | 21 (`Δ`=274.9 s, default) | 25 (`Δ`=4398 s) |
|---|---|---|---|
| 10³ /s | 3.6·10⁴ | 2.8·10⁵ | 4.4·10⁶ |
| 10⁵ /s | 3.6·10⁶ | 2.8·10⁷ | 4.4·10⁸ |
| 10⁶ /s | 3.6·10⁷ | 2.8·10⁸ | 4.4·10⁹ |

`𝒌` is an index-space quantity and therefore grows with throughput; the
time-space statement of Lemma 7 — *two values allocated more than `Δ + 2ε`
apart are always correctly ordered* — is invariant and is the one to reason
with. Choosing `𝑫` is exactly the trade: a larger `Δ` tolerates more clock
skew, a smaller `Δ` yields a tighter `𝒌`. The floor of the ladder bounds how
tight `𝒌` can be made: `𝑫 = 18` is the smallest the 96-bit layout admits, so
`𝑾 ≥ 34.4 s + 2ε` whatever the deployment's clocks are worth.

---

## 5. What is *not* claimed

* Global values are **not** linearizable, and cannot be: `Before` is computed
  from two identifiers alone, with no coordination, so a total order agreeing
  with real time would contradict the impossibility of lock-free consensus-free
  global timestamping under unsynchronized clocks. `𝒌`-orderedness is precisely
  the weakened guarantee that survives.
* Nothing here establishes **uniqueness** of global values beyond (A7): two
  processes that draw the same random `𝒍` and allocate in the same epoch with
  the same `𝒙ₗ` and `𝒔` collide. With 32-bit random `𝒍`, the birthday bound
  gives ≈ 65 000 allocators for a collision probability near ½ — the figure
  quoted in the README.
* Mixed comparison of local and global values is not addressed, and since v3
  cannot be written: `L` and `G` are distinct types and neither `Before`
  accepts the other. Convert with `L.ToG` / `G.ToL` first. The conversion
  preserves `⟨𝒕,𝒔⟩` exactly (Propositions 1 and 2 agree on those fields), so a
  set converted to one type compares under the theorems of §3 and §4.

---

## 6. The Lean 4 formalization

[proof.lean](proof.lean) formalizes §3 and §4 in Lean 4 (no Mathlib, no
Batteries; Lean 4 core `Nat` only). It contains:

| Lean name | this document |
|---|---|
| `Guid.digit_lt` | the key step of Lemma 2 |
| `Guid.div_lt_div_of_add_le` | the division step of Lemma 7 |
| `Guid.pack` | Proposition 1, the positional encoding |
| `Guid.pack_lt_of_epoch_lt` | Lemma 5 (epoch dominance) |
| `Guid.pack_le_of_suffix_le` | the equal-prefix case behind Corollary 2 |
| `Guid.Trace` | the system model of §4.1 with (A1), (A5), (A6) |
| `Guid.Trace.A_lt_of_epoch_lt` | Lemma 5 transported to the model |
| `Guid.Trace.no_inversion_of_time` | **Lemma 7**: far apart in time ⟹ ordered |
| `Guid.Trace.no_inversion` | Theorem 2, pair form: far apart in index ⟹ ordered |
| `Guid.Trace.k_ordered` | **Theorem 2 as stated**: `A[i−k] ≤ A[i] ≤ A[i+k]` |
| `Guid.Trace.inversion_window` | Lemma 7, contrapositive form |
| `Guid.Trace.displacement` | Corollary 1 |
| `Guid.sane` | a witness `Trace`, so the model is not vacuous |
| `Guid.packL` | Proposition 2, the local encoding |
| `Guid.packL_split` | Lemma 4, `⟦𝒗⟧ = 𝒅·2⁶¹ + 𝑽` |
| `Guid.packL_lt_of_lt` | **Lemma 4**: a monotone `𝑽` gives monotone values |
| `Guid.wrap_inverts` | the counterexample of §7: `𝒔 = 𝒏 mod 2¹⁴` is not monotone |
| `Guid.Local` | the allocator model of §3.2, with Lemma 3 as its field |
| `Guid.Local.sorted` | **Theorem 1**: local values sort in allocation order |
| `Guid.Local.distinct` | local values are pairwise distinct |

The model is deliberately *abstract in the clock and the allocation times*:
`tau`, `clk`, `node`, `seq` are arbitrary functions constrained only by the
skew and rate hypotheses, so the theorem holds for every execution of the
system, not for a particular schedule.

Check it with:

```bash
lean proof.lean      # Lean 4; no dependencies
```

**Status: verified** with Lean 4.34.0 (arm64-apple-darwin). The file emits no
errors and no warnings; its `#print axioms` directives report

```
'Guid.Trace.no_inversion_of_time' depends on axioms: [propext, Quot.sound]
'Guid.Trace.no_inversion'         depends on axioms: [propext, Quot.sound]
'Guid.Trace.k_ordered'            depends on axioms: [propext, Quot.sound]
'Guid.Trace.inversion_window'     depends on axioms: [propext, Quot.sound]
'Guid.Trace.displacement'         depends on axioms: [propext, Quot.sound]
'Guid.pack_lt_of_epoch_lt'        depends on axioms: [propext, Quot.sound]
'Guid.pack_le_of_suffix_le'       depends on axioms: [propext]
'Guid.sane'                       depends on axioms: [propext, Quot.sound]
'Guid.packL_lt_of_lt'             depends on axioms: [propext]
'Guid.wrap_inverts'               depends on axioms: [propext, Quot.sound]
'Guid.Local.sorted'               depends on axioms: [propext]
'Guid.Local.distinct'             depends on axioms: [propext, Quot.sound]
```

`propext` and `Quot.sound` are dependencies of the `omega` decision procedure.
The absence of `sorryAx` means nothing is assumed, and the absence of
`Classical.choice` means the proofs are constructive.

---

## 7. Where the assumptions bite

The proofs are only as good as (A1)–(A7). Two are properties of *this*
implementation rather than of the mathematics, and one hazard that earlier
versions of the library carried is recorded here because the fix is what §3
now rests on.

**(A1) — drift must be fixed cluster-wide.** `⟨𝒅⟩` is the most significant
field, so two values allocated at the same instant with different drift
settings are ordered by their drift, not their time.

v2 accepted the drift as an optional argument of every allocator, which put a
quantity that must be constant across a keyspace — and across the lifetime of
the data — at the call site, where it varies most easily. v3 binds it to the
clock instead: `WithDrift` configures it, [`Chronos.Drift`](../clock.go#L38)
reports it, and `NewG` / `NewL` read it from there. One clock is therefore one
drift by construction, and (A1) reduces to a statement about clocks:
`guid.NewClock(guid.WithDrift(60*time.Second))` and
`guid.NewClock(guid.WithDrift(10*time.Minute))` must not feed one keyspace.
The library cannot enforce that across processes, so (A1) remains an
assumption — but it is no longer one an allocation can violate on its own.

**(A2) — `⟨𝒕⟩` and `⟨𝒔⟩` must come from one allocator.** v2 let an application
supply `⟨𝒔⟩` from a generator independent of `⟨𝒕⟩` (`WithUnique`), which is
exactly the coupling Algorithm 1 exists to provide; such a history was outside
Theorem 1 unless the supplied generator was itself monotone and never folded
back while `⟨𝒕⟩` stood still. v3 removes the option: `Chronos.T` returns the
pair and no exported configuration decouples it, so (A2) holds by construction
for every clock the library builds. It survives as an assumption only for a
hand-written `Chronos`, whose `T` must return a pair `⟨𝒕,𝒔⟩` that strictly
increases in the sense of Lemma 3.

**The hazard Algorithm 1 removes.** Before the coupled allocator, `⟨𝒔⟩` came
from a free-running process-global counter, `𝒔 = 𝒏 mod 2¹⁴`, read separately
from `⟨𝒕⟩`. `𝒏` is strictly increasing but `𝒔` is not: it is monotone on each
block `[𝒎·2¹⁴, (𝒎+1)·2¹⁴)` and drops from `16383` to `0` at every block
boundary. Two allocations sharing a tick `𝒙` are ordered by `𝒔` alone, so a
block boundary falling between them inverts them.

The intuitive budget — *"the sequence is 14 bits, so a process staying under
`2¹⁴` allocations per tick preserves order"* — does not imply the required
property. Those are different quantities: the **count** per tick, versus
whether a multiple of `2¹⁴` is **crossed** between the first and last of them.
A tick holding only two allocations, numbered `𝒏 = 16383` and `𝒏 = 16384`, is
four orders of magnitude under budget and still inverted; a tick holding 16 000
allocations numbered `100 … 16 099` is perfectly ordered. The count bounds how
many crossings can occur, never where one occurs, and one is enough. This is
`Guid.wrap_inverts` in [proof.lean](proof.lean), stated for arbitrary `𝒅` and
`𝒙`.

Measured on the previous implementation: two allocations forced across the
boundary invert every time; 8 goroutines × 300 000 allocations produced 1026
inversions and one duplicate; a frozen clock with `3·2¹⁴` allocations produced
3 inversions and 32 769 duplicates. All three are zero under Algorithm 1.

**A second hazard removed with it.** `Chronos.T()` used to read `⟨𝒕⟩` and
`⟨𝒔⟩` as two separate operations. A goroutine descheduled between them pairs a
stale timestamp with a much later sequence number, which produced the duplicate
above at a rate four orders of magnitude below the `2¹⁴` budget. Algorithm 1
makes the pair a single atomic quantity and closes the window.

**Clock monotonicity is no longer required.** `unixtime` uses
`time.Now().UnixNano()`, a wall clock that NTP may step backwards. Under
Algorithm 1 a backwards step cannot invert anything: step 1 does not fire and
`𝑽` continues from its high water mark. What a backwards step now costs is
accuracy, not order — `Time(uid)` reports the high water mark until real time
catches up. NTP slewing is harmless either way.

One minor observation, not reachable through the public API: `Diff` subtracts
`𝒕` and `𝒔` independently and re-packs; the result is a well-formed value only
when `a.Time() ≥ b.Time()` and `a.Seq() ≥ b.Seq()`. It is an approximation by
its own documentation and is outside the scope of this note.
