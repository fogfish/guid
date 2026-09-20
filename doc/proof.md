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
[`makeG`](../global.go#L77) / [`makeL`](../local.go#L67) and the comparison operators
[`G.Before`](../global.go#L110) / [`L.Before`](../local.go#L83). §1 establishes that
layout; everything after that is arithmetic on a positional numeral system.

They cover [`makeX`](../extended.go#L139) too. `X` is the same schema at 128
bits — the same fractions in the same order, the same clock, sequencer and
drift ladder — with `⟨𝒍⟩` widened to 58 bits and 6 bits given over to the
RFC 9562 version and variant fields. §1.2′ discharges those two differences
once, after which every statement below holds for `X` verbatim.

The layout is unchanged from v2. What v3 changed is the *storage* of it: `G` is
the 96-bit number in big-endian bytes and `L` is the 64-bit number itself, two
distinct types rather than one 128-bit struct carrying either. §1.6 records
what that buys the proofs.

---

## 1. The encoding

### 1.1 Notation

| symbol | meaning | width |
|---|---|---|
| `𝑫`   | drift parameter, the rung's `Drift.Bits` value, `𝑫 ∈ {0,14,17,19,21,23,25,30}` | — |
| `𝒅`   | drift code stored in the value, the index of the rung, `𝒅 ∈ {0,…,7}` | 3 bit |
| `𝒕`   | clock reading in nanoseconds | 64 bit |
| `𝒙`   | truncated clock, `𝒙 = ⌊𝒕 / 2¹⁷⌋` (unit ≈ 131 µs) | 47 bit |
| `𝑬`   | *epoch*, `𝑬 = ⌊𝒙 / 2^𝑫⌋ = ⌊𝒕 / 2^(17+𝑫)⌋` | 47−𝑫 bit |
| `𝒙ₗ`  | low clock bits, `𝒙ₗ = 𝒙 mod 2^𝑫` | 𝑫 bit |
| `𝒍`   | node (allocator) identity | 32 bit in `G`, 58 bit in `X` |
| `𝒔`   | per-process sequence, `𝒔 = 𝒏 mod 2¹⁴` for the `𝒏`-th call | 14 bit |
| `Δ`   | **drift window** `Δ = 2^(17+𝑫)` nanoseconds | — |
| `⟦𝒖⟧` | numeric value of a k-ordered value as an integer; for `G` the 96-bit big-endian number its bytes hold, for `X` the 128-bit one, for `L` the 64-bit number itself | — |
| `𝒖.hi`, `𝒖.lo` | the words of a `G`, `⟦𝒖⟧ = 𝒖.hi·2⁶⁴ + 𝒖.lo`, see [`G.words`](../global.go#L91) | 32, 64 bit |
| `⟪𝒖⟫` | *payload* of an `X`, the 122 bits left once the two fields RFC 9562 fixes are removed, see §1.2′ | 122 bit |

`Δ` is the quantity the user configures, on the clock and not per allocation,
with `WithDrift`. `⟨𝒅⟩` is 3 bits, so the ladder has eight rungs while the
layout admits every `𝑫 ∈ {0,…,47}`; the rungs are therefore *chosen* rather
than contiguous. [`driftLadder`](../drift.go#L86) is that table and
[`DriftOf`](../drift.go#L107) maps a requested budget to the smallest rung
whose window covers it:

| `𝒅` | 0 | 1 | 2 | 3 | 4 *(default)* | 5 | 6 | 7 |
|---|---|---|---|---|---|---|---|---|
| `𝑫` | 0 | 14 | 17 | 19 | **21** | 23 | 25 | 30 |
| `Δ` | 131 µs | 2.15 s | 17.2 s | 68.7 s | **274.9 s** | 1099 s | 4398 s | 140737 s |
| constant | `Drift131us` | `Drift2s` | `Drift17s` | `Drift68s` | **`Drift275s`** | `Drift1099s` | `Drift4398s` | `Drift39h` |

The ladder is a property of `⟨𝒅⟩` rather than of any one type, so `L`, `G` and
`X` share it: the rung means the same thing and the code stored in a value has
the same reading whichever width the value is.

The ladder spends one rung below a second and seven above it. The floor
`𝑫 = 0` is where `⟨𝒙ₗ⟩` vanishes altogether and the field order
`⟨𝒅⟩·⟨𝑬⟩·⟨𝒍⟩·⟨𝒔⟩` *is* Snowflake's: the window is `2ε`, and a run is one tick
wide, so the rung offers ordering and no attribution at all. The seven above it
are failover budgets, which is what the library was written for, and they cover
the interval a hand-over can occupy — from a consensus election to a split
brain found the next morning. Two rungs are distinguishable only where the
wider `Δ` is large against `2ε`, which is why the sub-second range is not
subdivided: below a second the rung stops deciding `𝑾` and the deployment's
clocks decide it instead.

The bounds on `𝑫` are properties of the schema rather than of the machine word,
and both lie outside the ladder. Below, `𝑫 = 0` is the point where there are no
low clock bits left to place. Above, `𝑫 = 47` is where `⟨𝑬⟩` vanishes and `⟨𝒍⟩`
outranks time altogether, so `𝑫 = 46` is the last value that orders by time at
all. The range of the clock bounds nothing in between: `2^(47−𝑫)` epochs of
`2^(17+𝑫)` ns span `2⁶⁴` ns ≈ 584 years for *every* `𝑫`, a narrower `⟨𝑬⟩`
counting proportionally wider epochs. What stops the ladder at `𝑫 = 30` is the
meaning of `Δ` rather than the geometry: past a day the window is no longer a
failover budget but all of time — every value of a deployment falls in one
epoch, so the partition by `⟨𝒍⟩` discriminates nothing, while `𝒌 = ρ·𝑾` grows
without any return.

> Revisions of this note before the ladder was re-based gave a 22-bit epoch as
> the ceiling's reason, capping `𝑫` at 25. The span calculation above shows
> that reason was empty — the span is invariant in `𝑫` — and 25 was in fact
> inherited from v2, whose codes meant `𝑫 = 18 + code`.

> Versions of this library up to v3 floored the ladder at `𝑫 = 18`, and that
> floor *was* geometry: the value was assembled by hand-placed shifts that
> required the top `𝑫 − 18` bits of `⟨𝒍⟩` to fall above the `hi`/`lo` boundary
> of the two machine words, so a smaller `𝑫` — which spills `⟨𝑬⟩` across that
> boundary instead — was a layout the arithmetic could not express. Proposition
> 1 never depended on it; `makeG` now places each fraction at the bit position
> the schema gives it, and the floor is gone with the shifts that caused it.

**`𝒍` is drawn from a linearly ordered space, and the schema ranks by that
order.** This is a modelling decision worth making explicit, because every
result about the interior of an epoch depends on it. The node identity is not
an opaque tag that merely has to *differ* between allocators: it is an element
of `𝑳 = {0,…,2^𝑵−1}` under `<`, and Lemma 2 below makes that order a component
of the order on identifiers. A deployment is expected to take `𝒍` from a space
it already orders — position on a consistent-hash ring, a shard or partition
number, an index into a membership list — through
[`WithNodeID`](../clock.go#L164), so that adjacency in the key space *is*
adjacency in the topology.

Two consequences matter for reading Part II, and they are easy to conflate.

*Assigning `𝒍` is management plane; allocating a value is data plane.* The
lock-free, coordination-free claim of this note is about **allocation**:
producing an identifier costs one atomic increment and consults nobody (§3.2).
Fixing `𝒍` is a separate and earlier act, performed once per allocator, outside
the allocation path, by whatever already decides cluster membership. It may use
as much coordination as it likes — a hash ring, a config file, a lease, an
operator typing a number — without touching any theorem below, because no
theorem below runs at that time. The split is the ordinary one between a
control plane that hands out identities and a data plane that then runs without
asking it anything.

*Randomizing `𝒍` is a seeding policy, not a different schema.*
[`WithNodeRandom`](../clock.go#L197) is the default because it discharges (A7)
with no infrastructure at all. It selects *which* element of `𝑳` an allocator
occupies; it does not make `𝑳` unordered, nor `Before` non-deterministic, nor
the interior of an epoch unordered. Every result in Parts I and II holds
verbatim under either policy, and Lemma 5′ is stated without reference to one.
What the choice does change is two things outside the order: whether the
induced ranking of nodes carries meaning — assigned `𝒍` mirrors the topology, a
random `𝒍` gives an arbitrary but thereafter fixed permutation — and whether
(A7) is guaranteed or merely probable, which is the subject of §5.

### 1.2 Proposition 1 (global layout)

For `𝑫 ∈ {0,…,47}`, `𝒅 < 2³`, `𝒍 < 2³²`, `𝒔 < 2¹⁴`:

```
  ⟦makeG(𝒍, 𝑫, 𝒕, 𝒔)⟧ = 𝒅·2⁹³ + 𝑬·2^(46+𝑫) + 𝒍·2^(14+𝑫) + 𝒙ₗ·2¹⁴ + 𝒔
```

i.e. the 96-bit word is the concatenation

```
     3      47 − 𝑫          32           𝑫        14
   |---|--------------|------------|----------|--------|
    ⟨𝒅⟩      ⟨𝑬⟩          ⟨𝒍⟩        ⟨𝒙ₗ⟩      ⟨𝒔⟩
```

*Proof.* [`makeG`](../global.go#L77) is the identity written out. It computes
`𝒙 = 𝒕 ≫ 17`, `𝑬 = 𝒙 ≫ 𝑫`, `𝒙ₗ = 𝒙 mod 2^𝑫`, and calls
[`place`](../common.go#L73) once per fraction with the bit position the schema
gives it — `𝒅` at 93, `𝑬` at `46+𝑫`, `𝒍` at `14+𝑫`, `𝒙ₗ` at 14, `𝒔` at 0 —
then ORs the results and writes the pair out big-endian with `joinG`.

`place(𝒗, 𝒑)` returns the pair `(𝒗 ≫ (64−𝒑), 𝒗 ≪ 𝒑)` for `𝒑 < 64` and
`(𝒗 ≪ (𝒑−64), 0)` otherwise, which in both cases is the base-2⁶⁴ decomposition
of `𝒗·2^𝒑`: Go defines a shift by 64 or more as zero, so `𝒑 = 0` and `𝒑 ≥ 64`
need no separate case, and a fraction that straddles the `hi`/`lo` boundary is
placed by the same expression as one that does not. Hence each call contributes
exactly `𝒗·2^𝒑` to `hi·2⁶⁴ + lo`.

The occupied ranges are pairwise disjoint and contiguous, since the widths sum
to `3 + (47−𝑫) + 32 + 𝑫 + 14 = 96` for every `𝑫 ≤ 47` and each field is placed
immediately above the one below it. Disjoint ranges make the OR an addition, so

```
  ⟦makeG⟧ = 𝒅·2⁹³ + 𝑬·2^(46+𝑫) + 𝒍·2^(14+𝑫) + 𝒙ₗ·2¹⁴ + 𝒔
```

which is the claim. The bounds `𝑬 < 2^(47−𝑫)` and `𝒙ₗ < 2^𝑫` hold by
construction from a 64-bit `𝒕`, and `𝒍`, `𝒔` are masked to their widths. ∎

The word boundary has left the argument entirely — it is a fact about the
registers `hi` and `lo` are held in, not about the value, which is why the
range of the proposition is now the whole of `{0,…,47}`. `Guid.pack_lt_96` in
[proof.lean](proof.lean) is the machine-checked half of the same statement:
the five fields fit 96 bits for every `𝑫 ≤ 47`.

`G.Time` and `G.Node` invert the packing with
[`extract`](../common.go#L83), reading the field at the same position and
width, so they are correct by the same argument.

> This proposition is also verified differentially against the implementation
> by [`TestPropositionG`](../drift_test.go), for every rung of the ladder over
> 160 000 random `(𝒍, 𝒕, 𝒔)` triples: the `big.Int` model of the positional
> form and `⟦makeG(...)⟧` agree bit for bit, and `Time`, `Node`, `Seq` return
> the fields that went in. Five of the eight rungs have `𝑫 < 18`, where `⟨𝑬⟩`
> spills across the word boundary, so the regime that the old layout could not
> express is the one the test mostly exercises.

### 1.2′ Proposition 1′ (extended layout)

`X` is the same schema at 128 bits, laid out so that the value is also a
well-formed RFC 9562 UUID of version 8. Two things change and nothing else
does: `⟨𝒍⟩` widens from 32 to 58 bits, and 6 of the 128 bits are spent on the
version and variant fields the RFC fixes.

Write `⟪𝒖⟫` for the **payload**, the 122-bit positional number the five
fractions occupy. For `𝑫 ∈ {0,…,47}`, `𝒅 < 2³`, `𝒍 < 2⁵⁸`, `𝒔 < 2¹⁴`:

```
  ⟪makeX(𝒍, 𝑫, 𝒕, 𝒔)⟫ = 𝒅·2¹¹⁹ + 𝑬·2^(72+𝑫) + 𝒍·2^(14+𝑫) + 𝒙ₗ·2¹⁴ + 𝒔
```

i.e. the payload is the concatenation

```
     3      47 − 𝑫            58             𝑫        14
   |---|--------------|------------------|----------|--------|
    ⟨𝒅⟩      ⟨𝑬⟩             ⟨𝒍⟩           ⟨𝒙ₗ⟩       ⟨𝒔⟩
```

and the value is that payload with the two constant fields spliced in:

```
  payload bit 𝒑  ↦  value bit  𝒑        for 𝒑 ∈ [0, 47]
                               𝒑 + 4    for 𝒑 ∈ [48, 59]
                               𝒑 + 6    for 𝒑 ∈ [60, 121]
```

counting from the most significant bit, as RFC 9562 does; value bits 48…51 are
the version `0b1000` and bits 64…65 the variant `0b10`.

*Proof of the payload identity.* [`makeX`](../extended.go#L139) does not place
fractions at computed positions the way `makeG` does. It transcribes
`Guid.pack` of [proof.lean](proof.lean),

```lean
def pack (D d E l x s : Nat) : Nat :=
  (((d * 2 ^ (47 - D) + E) * 2 ^ 32 + l) * 2 ^ D + x) * 2 ^ 14 + s
```

with `2³²` replaced by `2⁵⁸`, and evaluates it in Horner form over a 128-bit
accumulator: [`horner`](../extended.go#L161) computes `𝒗·2^𝒌 + 𝒂` for `𝒂 < 2^𝒌`
on the pair `(hi, lo)`, and the four calls supply `(47−𝑫, 𝑬)`, `(58, 𝒍)`,
`(𝑫, 𝒙ₗ)`, `(14, 𝒔)` in that order starting from `𝒅`. Expanding the Horner form
*is* the claimed sum, so the proposition holds by the definition rather than by
an argument about bit placement — which is the reason for writing it this way.
The accumulator holds 3, then `50−𝑫`, `108−𝑫`, `108` and finally 122 bits, so
no step overflows 128, by the same width identity as before:
`3 + (47−𝑫) + 58 + 𝑫 + 14 = 122`. ∎

The width half of this is machine-checked. `Guid.pack` in
[proof.lean](proof.lean) takes the node width `𝑵` as a parameter, `Guid.pack_lt`
proves the layout occupies `64 + 𝑵` bits for every `𝑫 ≤ 47`, and
`Guid.pack_lt_122` is that at `𝑵 = 58` — the payload of an `X` is 122 bits,
leaving exactly the 6 the RFC reserves. Because the whole development is
abstract in `𝑵`, Part I and Part II hold for `X` by the same proof terms that
prove them for `G`; see §6.

*Proof of the splice.* [`splice`](../extended.go#L177) cuts the payload into
the three runs the two reserved fields leave — `[0,62)`, `[62,74)`, `[74,122)`
counted upward from the least significant bit — and places each with
[`place`](../common.go#L73) at `0`, `64` and `80`, alongside the version at
`76` and the variant at `62`. By the argument of §1.2 each call contributes
exactly `𝒗·2^𝒑`, and the five ranges are pairwise disjoint and together cover
all 128 bits, so the OR is an addition and the map above is realised exactly.
It is a permutation of bit positions and a pair of constants; it does not
depend on `𝑫`. [`X.payload`](../extended.go#L189) is the same three placements
in reverse, so `⟪·⟫` is recovered exactly. ∎

**The version and variant are order-transparent.** This is what has to be shown
for §1.4–§1.5 to carry over, since the two fields interrupt the field layout of
a schema whose whole premise is that byte order is sort order.

> *Claim.* For values `𝒂`, `𝒃` of `X`, `⟦𝒂⟧ < ⟦𝒃⟧ ⟺ ⟪𝒂⟫ < ⟪𝒃⟫`.
>
> *Proof.* Compare most significant bit first. At every version or variant
> position the two values hold the same constant — they are constants of the
> format, present in every value — so the comparison neither concludes nor
> reverses there and falls through to the next position. What remains is the
> comparison of the payload bits in their original relative order, which is
> lexicographic comparison of `⟪𝒂⟫` against `⟪𝒃⟫`. ∎

Hence Lemma 1 holds for `X` — [`X.Before`](../extended.go#L222) compares the
pair `(hi, lo)` lexicographically, which is numeric order on `⟦·⟧`, which by
the claim is numeric order on `⟪·⟫` — and so does Lemma 2, over the fields
`(𝑬, 𝒍, 𝒙ₗ, 𝒔)` of the payload. `bytes.Compare` over the raw 16 bytes agrees
with both, as for `G`.

Everything in Parts I and II therefore applies to `X` unchanged, with `2³²`
replaced by `2⁵⁸` in the residue bound of §4.2: the field order
`⟨𝒅⟩ ≻ ⟨𝑬⟩ ≻ ⟨𝒍⟩ ≻ ⟨𝒙ₗ⟩ ≻ ⟨𝒔⟩` is the same, the clock and sequencer are the
same object, and the drift ladder is the same table. In the Lean development
this is not an analogy but the same theorem: the node width is a parameter
there, so `k_ordered` and the rest are proved once and instantiated at 32 bits
for `G` and 58 for `X`. The 26 extra bits of `⟨𝒍⟩`
are not part of any ordering claim; they move the birthday bound of §5 from
≈ 6.5·10⁴ allocators to ≈ 5.4·10⁸, which is a statement about *uniqueness*, the
thing this note does not prove.

> The payload identity and the splice are verified differentially against the
> implementation by [`TestPropositionX`](../extended_test.go), for every rung
> of the ladder over 160 000 random `(𝒍, 𝒕, 𝒔)` triples, against a `big.Int`
> model written from the schema rather than from the code; order transparency
> is checked directly by `TestOrderTransparency`, which compares `Before`,
> `After` and `bytes.Compare` against the ordering of the modelled payload.
> That the result is a UUID every other system accepts is checked by
> `TestRFC9562` on the reserved fields and the canonical string.

### 1.3 Proposition 2 (local layout)

```
  ⟦makeL(𝑫, 𝒕, 𝒔)⟧ = 𝒅·2⁶¹ + 𝒙·2¹⁴ + 𝒔
```

*Proof.* Immediate from [`makeL`](../local.go#L67): `d = 𝒅 ≪ 61`,
`x = 𝒕 ≫ 17 ≪ 14 = 𝒙·2¹⁴`, `seq = 𝒔 < 2¹⁴`; the three ranges are disjoint and
contiguous, `3 + 47 + 14 = 64`. ∎

Note that the local value keeps the *whole* `𝒙` above `𝒔` — it is not split
around a node field, because there is no node field.

### 1.4 Lemma 1 (order homomorphism)

`Before(a, b) ⟺ ⟦a⟧ < ⟦b⟧`.

*Proof.* For `L`, [`Before`](../local.go#L83) is `uint64` comparison and `⟦·⟧` is
the identity, so the claim is trivial. For `G`,
[`Before`](../global.go#L110) is `a.hi < b.hi ∨ (a.hi = b.hi ∧ a.lo < b.lo)`, which
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
  property of the *value*, not of the type. A global value with the lowest
  drift and `𝑬 = 0` also has `hi = 0`; §7 had to argue that the ladder never
  reached that rung to rule it out. The types carry the distinction now, so the
  argument is no longer load-bearing — the bottom of the ladder was re-enabled
  once it was not, and then, with the shift arithmetic replaced by positional
  placement (§1.2), extended down to the millisecond rungs of §1.1.

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
* **(A7) Distinct, ordered, statically assigned identities.** `𝒍` is an element
  of the linearly ordered space `𝑳 = {0,…,2^𝑵−1}` (§1.1), fixed once per
  allocator before it allocates and constant thereafter, and distinct processes
  use distinct `𝒍`. *How* the assignment is made is out of scope — it is
  management plane, and the results below use only the fact that it happened,
  never the policy that made it happen.

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

For `X` the same computation runs over the payload with `2³²` replaced by
`2⁵⁸` and `2⁹³`, `2^(46+𝑫)` by `2¹¹⁹`, `2^(72+𝑫)`: the residue is again exactly
one below the weight of the epoch, because the widths below `⟨𝑬⟩` sum to it by
construction. `Guid.pack_lt_of_epoch_lt` is this lemma machine-checked, and it
is stated over a node of `𝑵` bits, so it is one theorem for both types.

This is *the* structural fact of the schema: the node identity outranks the low
`𝑫` bits of the clock, but nothing outranks the epoch. Clock disagreement can
permute values only *inside* an epoch.

### 4.2′ Lemma 5′ (intra-epoch structure)

Lemma 5 is the negative half — what cannot be overturned. This is the positive
half: what the order *is* where Lemma 5 stops applying.

*Assume (A1), (A7). If `𝑬(𝒖) = 𝑬(𝒗)` then*

```
  ⟦𝒖⟧ < ⟦𝒗⟧   ⟺   (𝒍ᵤ, 𝒙ₗᵤ, 𝒔ᵤ)  <ₗₑₓ  (𝒍ᵥ, 𝒙ₗᵥ, 𝒔ᵥ) .
```

*In particular the values of one epoch are **totally ordered**, and they are
grouped into contiguous runs — one per allocator, the runs ordered by `𝒍`.*

*Proof.* With `𝒅` equal by (A1) and `𝑬` equal by hypothesis, the two leading
fields contribute the same value to both packings, so by Lemma 2 the
lexicographic comparison on `(𝒅, 𝑬, 𝒍, 𝒙ₗ, 𝒔)` reduces to the lexicographic
comparison on the suffix `(𝒍, 𝒙ₗ, 𝒔)`; that reduction is the equivalence.
Totality is then totality of `<ₗₑₓ` on a product of linear orders, which `𝑳` is
by (A7). For contiguity, let `𝒍ᵤ = 𝒍ᵥ` and `⟦𝒖⟧ < ⟦𝒘⟧ < ⟦𝒗⟧`. By Lemma 5,
`𝑬(𝒘) < 𝑬(𝒖)` would force `⟦𝒘⟧ < ⟦𝒖⟧` and `𝑬(𝒘) > 𝑬(𝒗)` would force
`⟦𝒗⟧ < ⟦𝒘⟧`, so `𝒘` shares the epoch and the equivalence applies to it, giving
`𝒍ᵤ ≤ 𝒍_𝒘 ≤ 𝒍ᵥ = 𝒍ᵤ`. ∎

`Guid.pack_lt_of_node_lt` is the forward direction machine-checked, and like
Lemma 5 it is stated over a node of `𝑵` bits, so it covers both types.

It is worth being explicit about what Lemmas 5 and 5′ jointly rule out, because
the negative framing of §4.4–§4.6 invites a stronger reading than the theorems
support. Across epochs, time decides. Within an epoch, `𝒍` decides — and
*decides* is the operative word. The order is total, and it is a function of
the two values alone: by Lemma 1, `Before` reads only the identifiers, so any
two of them compare the same way for every observer, at every separation, on
every machine. **No result in Part II says that identifiers allocated close
together are unordered, ambiguously ordered, or ordered differently by
different readers.** What §4.4–§4.6 bound is a different quantity entirely: how
far this fixed order can diverge from the order of allocation in real time.

Nor does the seeding policy enter. Lemma 5′ never asks where `𝒍` came from, only
that it is fixed and comparable (A7); a random draw chooses the node's place in
`𝑳` rather than removing the order from `𝑳`. Randomness changes which
permutation of the allocators the key space exhibits, not whether it exhibits
one.

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

Within one epoch this is the `𝒍ᵤ = 𝒍ᵥ` case of Lemma 5′; the corollary extends
it across epochs and along the whole trace rather than a pair, which is what
the appeal to Lemmas 3 and 4 buys.

So the global sequence is an `𝑵`-way **merge of `𝑵` individually sorted
streams**, and every inversion is a cross-node inversion. Theorem 2 bounds how
far the merge can be off; Corollary 2 says the disorder is entirely inter-node.

Operationally this corollary, not Theorem 2, is the one the library is built
around. Combined with Lemma 5′ it says that inside one epoch the key space is
*partitioned by allocator*: the contiguity clause gives each node's values as a
contiguous run, this corollary sorts each run exactly, and the runs are ordered
by `𝒍` — which is why §1.1 asks that `𝒍` be taken from a space whose order
means something. That is the property a leader-follower hand-over needs.
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

| `ρ` \ `𝑫` | 0 (`Δ`=131 µs, floor) | 17 (`Δ`=17.2 s) | 21 (`Δ`=274.9 s, default) | 25 (`Δ`=4398 s) | 30 (`Δ`=39.1 h, top) |
|---|---|---|---|---|---|
| 10³ /s | 2.0·10³ | 1.9·10⁴ | 2.8·10⁵ | 4.4·10⁶ | 1.4·10⁸ |
| 10⁵ /s | 2.0·10⁵ | 1.9·10⁶ | 2.8·10⁷ | 4.4·10⁸ | 1.4·10¹⁰ |
| 10⁶ /s | 2.0·10⁶ | 1.9·10⁷ | 2.8·10⁸ | 4.4·10⁹ | 1.4·10¹¹ |

`𝒌` is an index-space quantity and therefore grows with throughput; the
time-space statement of Lemma 7 — *two values allocated more than `Δ + 2ε`
apart are always correctly ordered* — is invariant and is the one to reason
with. Choosing `𝑫` is exactly the trade: a larger `Δ` tolerates more clock
skew, a smaller `Δ` yields a tighter `𝒌`. The floor of the ladder bounds how
tight `𝒌` can be made: `𝑫 = 0` is its lowest rung, so `𝑾 ≥ 131 µs + 2ε`
whatever the deployment's clocks are worth.

The floor rung shifts where the window comes from. At `𝑫 = 0` with `ε = 1 s`
the schema contributes 131 µs of a 2.000131 s window — `𝑾` is `2ε` to within
0.007 %, so `𝒌` is decided by the quality of the deployment's clocks and not by
the drift setting at all. Selecting it is therefore a statement about `ε`: it
pays off in a datacenter where `ε` is a millisecond (`𝑾 ≈ 2 ms`) and buys
nothing where `ε` is a second. This is also why the ladder has one rung there
rather than four: two rungs whose `Δ` both sit far below `2ε` produce the same
`𝑾`, so they are the same rung in everything but name. This is a change in what a user has to
reason about, not in the theorem — §4.8 shows `𝑾 = Δ + 2ε` is tight either
way.

---

## 5. What is *not* claimed

* Global values are **not** linearizable, and cannot be: `Before` is computed
  from two identifiers alone, with no coordination, so a total order agreeing
  with real time would contradict the impossibility of lock-free consensus-free
  global timestamping under unsynchronized clocks. `𝒌`-orderedness is precisely
  the weakened guarantee that survives. Read the emphasis carefully: what fails
  is *agreeing with real time*, not *total order*. Global values are totally
  ordered at every separation — across epochs by Lemma 5, within one by Lemma
  5′ — and the order is reproducible by anyone holding the two values. The
  failure is that this total order is not the real-time one, and `𝑾` bounds the
  gap between them. A reader who takes "not linearizable" to mean "comparison
  is unsettled inside the window" has read one word for the other.
* Nothing here establishes **uniqueness** of global values beyond (A7): two
  processes that draw the same random `𝒍` and allocate in the same epoch with
  the same `𝒙ₗ` and `𝒔` collide. With 32-bit random `𝒍`, the birthday bound
  gives ≈ 65 000 allocators for a collision probability near ½ — the figure
  quoted in the README. `X` widens `𝒍` to 58 bits and moves that bound to
  ≈ 5.4·10⁸, which is the reason the type exists: ordering is proven here,
  uniqueness is the assumption, and widening `𝒍` does more for it than anything
  in this note. Two allocators sharing an `𝒍` still only collide if they also
  agree on `⟨𝑬, 𝒙ₗ, 𝒔⟩`, so the true rate is far below the bare birthday figure
  — but the bare figure is the one to quote, because two allocators that share
  `𝒍` will eventually agree on `⟨𝒕,𝒔⟩` too.
* Mixed comparison of values of different width is not addressed, and since v3
  cannot be written: `L`, `G` and `X` are distinct types and no `Before`
  accepts another. Convert first, by building the destination — `G.FromL`,
  `G.FromX`, `L.FromG`, `L.FromX`, `X.FromG`, `X.FromL`. Every conversion
  preserves `⟨𝒕,𝒔⟩` exactly — Propositions 1, 1′ and 2 agree on those fields —
  so a set converted to one type compares under the theorems of §3 and §4.
  `G.FromX` additionally truncates `𝒍` to 32 bits, which is a statement about
  uniqueness, not about order.

---

## 6. The Lean 4 formalization

[proof.lean](proof.lean) formalizes §3 and §4 in Lean 4 (no Mathlib, no
Batteries; Lean 4 core `Nat` only). It contains:

| Lean name | this document |
|---|---|
| `Guid.digit_lt` | the key step of Lemma 2 |
| `Guid.div_lt_div_of_add_le` | the division step of Lemma 7 |
| `Guid.pack` | Propositions 1 and 1′, the positional encoding, over a node of `N` bits |
| `Guid.ladder` | the drift ladder of §1.1, code `𝒅` ↦ `𝑫` |
| `Guid.ladder_le` | every rung satisfies `𝑫 ≤ 30`, so `⟨𝑬⟩` keeps 17 bits |
| `Guid.ladder_mono` | the ladder ascends, so `⟨𝒅⟩` orders values by window |
| `Guid.pack_lt` | **the layout fits `64 + N` bits at every `𝑫 ≤ 47`** (§1.2, §1.2′) |
| `Guid.pack_lt_96` | the same at `𝑵 = 32`: a `G` is 96 bits |
| `Guid.pack_lt_122` | the same at `𝑵 = 58`: the payload of an `X` is 122 bits |
| `Guid.Trace.D_le` | a rung of the ladder never exceeds the 47 bits of `⟨𝒙⟩` |
| `Guid.Trace.A_lt` | every value of a trace is a `64 + N` bit number |
| `Guid.Trace.A_lt_96` | the same at `𝑵 = 32`, for `G` |
| `Guid.Trace.A_lt_122` | the same at `𝑵 = 58`, for the payload of `X` |
| `Guid.pack_lt_of_epoch_lt` | Lemma 5 (epoch dominance) |
| `Guid.pack_lt_of_node_lt` | Lemma 5′ (intra-epoch structure), forward direction |
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

It is also abstract in **the width of the node field**. `Guid.pack` takes that
width as a parameter `N` and `Trace` carries it, so nothing in Part I or Part
II is stated about 32 bits in particular: `k_ordered`, `no_inversion`,
`displacement` and the rest hold at `𝑵 = 32` for `guid.G` and at `𝑵 = 58` for
`guid.X` by the same proof term. Only the width bounds name a number, and both
numbers are checked — `pack_lt_96` and `pack_lt_122`. This is the whole of what
`X` needed from the formalization; the RFC 9562 splice lives outside it,
because it is a permutation of bit positions and a pair of constants, which
§1.2′ shows changes no order.

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
'Guid.Trace.A_lt_96'              depends on axioms: [propext, Quot.sound]
'Guid.pack_lt_96'                 depends on axioms: [propext, Quot.sound]
'Guid.ladder_le'                  does not depend on any axioms
'Guid.ladder_mono'                does not depend on any axioms
'Guid.pack_lt_of_epoch_lt'        depends on axioms: [propext, Quot.sound]
'Guid.pack_lt_of_node_lt'         depends on axioms: [propext, Quot.sound]
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
clock instead: `WithDrift` configures it, [`Chronos.Drift`](../clock.go#L56)
reports it, and `NewG` / `NewL` read it from there. One clock is therefore one
drift by construction, and (A1) reduces to a statement about clocks:
`guid.NewClock(guid.WithDrift(guid.Drift17s))` and
`guid.NewClock(guid.WithDrift(guid.Drift1099s))` must not feed one keyspace.
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
