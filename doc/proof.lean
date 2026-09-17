/-
  guid — k-orderedness of globally allocated identifiers, in Lean 4.

  This file is the machine-checked counterpart of §3 and §4 of `prove.md`.
  It proves two things.

    Theorem (local ordering).  Because `sequence` (sequence.go) allocates the
    pair ⟨t,s⟩ as a single word that is only ever incremented or raised, local
    values sort exactly in allocation order -- at any rate, under any clock.
    The scheme it replaces is shown to be non-monotone by counterexample.

  and:

    Theorem (k-orderedness).  If the cluster performs at most `k` allocations in
    any real-time window of length `W = Δ + 2ε`, and every node's clock is
    within `ε` of real time, then the sequence `A` of allocated 96-bit values,
    indexed by real allocation time, satisfies

        A[i - k] ≤ A[i] ≤ A[i + k]     for all i with k < i.

  The proof rests on one structural fact about the bit layout produced by
  `makeG` (guid.go) — the epoch field ⟨E⟩ = ⌊t / 2^(17+D)⌋ outranks the node
  field ⟨l⟩ — and on one arithmetic fact about clocks — two readings more than
  Δ apart fall into different epochs.

  Only Lean 4 core is used; there are no dependencies.  Check with:

      lean prove.lean

  Naming: `delta` = Δ, `eps` = ε, `tau i` = τᵢ, `clk i` = cᵢ.
-/

namespace Guid

/-! ## 0.  Arithmetic helpers -/

/-- `0 < 2 ^ n`. -/
theorem two_pow_pos (n : Nat) : 0 < 2 ^ n := by
  induction n with
  | zero => decide
  | succ n ih =>
    have h : 2 ^ (n + 1) = 2 ^ n * 2 := by rw [Nat.pow_succ]
    omega

/-- The step of the positional-comparison lemma (`prove.md`, Lemma 2):
    a more significant digit dominates everything below it. -/
theorem digit_lt {b p q r : Nat} (hpq : p < q) (hr : r < b) : p * b + r < q * b := by
  have hpq' : p + 1 ≤ q := hpq
  have hexp : (p + 1) * b = p * b + b := by rw [Nat.add_mul, Nat.one_mul]
  have hle : (p + 1) * b ≤ q * b := Nat.mul_le_mul hpq' (Nat.le_refl b)
  omega

/-- Division by a positive `d` is strictly monotone across a full step: two
    readings at least `d` apart land in different buckets.
    (`prove.md`, the division step of Lemma 7.) -/
theorem div_lt_div_of_add_le {a b d : Nat} (hd : 0 < d) (h : a + d ≤ b) :
    a / d < b / d := by
  have ha : d * (a / d) + a % d = a := Nat.div_add_mod a d
  have hb : d * (b / d) + b % d = b := Nat.div_add_mod b d
  have ham : a % d < d := Nat.mod_lt a hd
  have hbm : b % d < d := Nat.mod_lt b hd
  cases Nat.lt_or_ge (a / d) (b / d) with
  | inl hlt => exact hlt
  | inr hge =>
    exfalso
    have hge' : b / d ≤ a / d := hge
    have hmul : d * (b / d) ≤ d * (a / d) := Nat.mul_le_mul (Nat.le_refl d) hge'
    omega

/-! ## 1.  The encoding

`pack` is the 96-bit layout established in `prove.md`, Proposition 1:

```
     3      47 - D          32           D        14
   |---|--------------|------------|----------|--------|
    ⟨d⟩      ⟨E⟩          ⟨l⟩        ⟨xlo⟩      ⟨s⟩
```
-/

/-- The value of a global identifier as a natural number, read off its fields:
    drift code `d`, epoch `E`, node `l`, low clock bits `x`, sequence `s`. -/
def pack (D d E l x s : Nat) : Nat :=
  (((d * 2 ^ (47 - D) + E) * 2 ^ 32 + l) * 2 ^ D + x) * 2 ^ 14 + s

/-- **Epoch dominance** (`prove.md`, Lemma 5).  A smaller epoch makes a smaller
    identifier, whatever the node, the low clock bits and the sequence are.
    This is the whole reason clock disagreement cannot reorder across epochs. -/
theorem pack_lt_of_epoch_lt {D d E l x s E' l' x' s' : Nat}
    (hl : l < 2 ^ 32) (hx : x < 2 ^ D) (hs : s < 2 ^ 14) (hE : E < E') :
    pack D d E l x s < pack D d E' l' x' s' := by
  have h0 : d * 2 ^ (47 - D) + E < d * 2 ^ (47 - D) + E' := by omega
  have h1 : (d * 2 ^ (47 - D) + E) * 2 ^ 32 + l
          < (d * 2 ^ (47 - D) + E') * 2 ^ 32 + l' :=
    Nat.lt_of_lt_of_le (digit_lt h0 hl) (Nat.le_add_right _ _)
  have h2 : ((d * 2 ^ (47 - D) + E) * 2 ^ 32 + l) * 2 ^ D + x
          < ((d * 2 ^ (47 - D) + E') * 2 ^ 32 + l') * 2 ^ D + x' :=
    Nat.lt_of_lt_of_le (digit_lt h1 hx) (Nat.le_add_right _ _)
  exact Nat.lt_of_lt_of_le (digit_lt h2 hs) (Nat.le_add_right _ _)

/-- Equal prefix: identifiers of the same node in the same epoch are ordered by
    `(xlo, s)` alone.  This is the step behind Corollary 2 of `prove.md` — the
    per-node stream is exactly sorted. -/
theorem pack_le_of_suffix_le {D d E l x s x' s' : Nat}
    (h : x * 2 ^ 14 + s ≤ x' * 2 ^ 14 + s') :
    pack D d E l x s ≤ pack D d E l x' s' := by
  have key : ∀ P a b : Nat,
      (P * 2 ^ D + a) * 2 ^ 14 + b = P * 2 ^ D * 2 ^ 14 + (a * 2 ^ 14 + b) := by
    intro P a b
    rw [Nat.add_mul, Nat.add_assoc]
  show ((( d * 2 ^ (47 - D) + E) * 2 ^ 32 + l) * 2 ^ D + x) * 2 ^ 14 + s
     ≤ (((d * 2 ^ (47 - D) + E) * 2 ^ 32 + l) * 2 ^ D + x') * 2 ^ 14 + s'
  rw [key, key]
  exact Nat.add_le_add_left h _

/-! ## 2.  The system model

An execution of the whole cluster.  `tau i` is the (unobservable) real time of
the `i`-th allocation, `clk i` the local clock reading that allocation actually
used.  Nothing is assumed about how many nodes there are, how they are
scheduled, or how their identities are chosen: `tau`, `clk`, `node` and `seq`
are arbitrary functions subject only to the skew and rate hypotheses.  The
theorem therefore holds for every execution, not for a particular schedule.
-/

structure Trace where
  /-- drift parameter `D ∈ {18,…,25}`, the value of `driftInBits`. -/
  D : Nat
  hD : 18 ≤ D
  /-- the drift window `Δ = 2^(17+D)` nanoseconds. -/
  delta : Nat
  hdelta : delta = 2 ^ (17 + D)
  /-- clock skew bound `ε`. -/
  eps : Nat
  /-- the `k` of k-orderedness. -/
  k : Nat
  /-- real time of the `i`-th allocation. -/
  tau : Nat → Nat
  /-- local clock reading used by the `i`-th allocation. -/
  clk : Nat → Nat
  /-- 32-bit allocator identity of the `i`-th allocation. -/
  node : Nat → Nat
  /-- 14-bit sequence of the `i`-th allocation. -/
  seq : Nat → Nat
  hnode : ∀ i, node i < 2 ^ 32
  hseq : ∀ i, seq i < 2 ^ 14
  /-- allocations are indexed in real-time order. -/
  mono_tau : ∀ i j, i ≤ j → tau i ≤ tau j
  /-- (A5) no clock runs more than `ε` ahead of real time. -/
  skew_hi : ∀ i, clk i ≤ tau i + eps
  /-- (A5) no clock runs more than `ε` behind real time. -/
  skew_lo : ∀ i, tau i ≤ clk i + eps
  /-- (A6) at most `k` allocations happen in any window of length `Δ + 2ε`;
      equivalently, `k` allocations apart in index means `Δ + 2ε` apart in time. -/
  rate : ∀ i j, i + k ≤ j → tau i + (delta + 2 * eps) ≤ tau j

namespace Trace

/-- The epoch of the `i`-th allocation, `E = ⌊t / 2^(17+D)⌋` (`prove.md`, Lemma 6). -/
def epoch (T : Trace) (i : Nat) : Nat := T.clk i / T.delta

/-- The low `D` clock bits of the `i`-th allocation, `xlo = ⌊t / 2^17⌋ mod 2^D`. -/
def tlo (T : Trace) (i : Nat) : Nat := T.clk i / 2 ^ 17 % 2 ^ T.D

/-- `A i` — the identifier produced by the `i`-th allocation, as a number. -/
def A (T : Trace) (i : Nat) : Nat :=
  pack T.D (T.D - 18) (T.epoch i) (T.node i) (T.tlo i) (T.seq i)

theorem tlo_lt (T : Trace) (i : Nat) : T.tlo i < 2 ^ T.D :=
  Nat.mod_lt (T.clk i / 2 ^ 17) (two_pow_pos T.D)

theorem delta_pos (T : Trace) : 0 < T.delta := by
  rw [T.hdelta]
  exact two_pow_pos _

/-- Lemma 5, transported to the model. -/
theorem A_lt_of_epoch_lt (T : Trace) {i j : Nat} (h : T.epoch i < T.epoch j) :
    T.A i < T.A j := by
  show pack T.D (T.D - 18) (T.epoch i) (T.node i) (T.tlo i) (T.seq i)
     < pack T.D (T.D - 18) (T.epoch j) (T.node j) (T.tlo j) (T.seq j)
  exact pack_lt_of_epoch_lt (T.hnode i) (T.tlo_lt i) (T.hseq i) h

/-! ## 3.  The theorems -/

/-- **Lemma 7** (`prove.md`).  Two allocations more than `Δ + 2ε` apart in real
    time are always correctly ordered — regardless of which nodes made them.
    This is the time-space statement; it does not mention `k`. -/
theorem no_inversion_of_time (T : Trace) {i j : Nat}
    (hfar : T.tau i + (T.delta + 2 * T.eps) ≤ T.tau j) : T.A i ≤ T.A j := by
  have hi := T.skew_hi i
  have hj := T.skew_lo j
  have hclk : T.clk i + T.delta ≤ T.clk j := by omega
  have hE : T.epoch i < T.epoch j := div_lt_div_of_add_le T.delta_pos hclk
  exact Nat.le_of_lt (T.A_lt_of_epoch_lt hE)

/-- Two allocations at least `k` apart in index are correctly ordered. -/
theorem no_inversion (T : Trace) {i j : Nat} (h : i + T.k ≤ j) : T.A i ≤ T.A j :=
  T.no_inversion_of_time (T.rate i j h)

/-- **Theorem 2** — the sequence of globally allocated identifiers is k-ordered:

      `A[i - k] ≤ A[i] ≤ A[i + k]`  for all `i` with `k < i`.

    (The upper index condition `i ≤ n - k` of the informal statement is the
    requirement that `i + k` be an allocation that happened; here the trace is
    infinite, so only `k < i` is needed.) -/
theorem k_ordered (T : Trace) {i : Nat} (hi : T.k < i) :
    T.A (i - T.k) ≤ T.A i ∧ T.A i ≤ T.A (i + T.k) := by
  constructor
  · exact T.no_inversion (by omega)
  · exact T.no_inversion (Nat.le_refl _)

/-- Contrapositive of Lemma 7: every inversion is confined to one drift window. -/
theorem inversion_window (T : Trace) {i j : Nat} (hinv : T.A j < T.A i) :
    T.tau j < T.tau i + (T.delta + 2 * T.eps) := by
  cases Nat.lt_or_ge (T.tau j) (T.tau i + (T.delta + 2 * T.eps)) with
  | inl h => exact h
  | inr h =>
    exfalso
    have h' : T.tau i + (T.delta + 2 * T.eps) ≤ T.tau j := h
    have := T.no_inversion_of_time h'
    omega

/-- **Corollary 1** (bounded displacement).  Every inversion is confined to `k`
    index positions: if a later allocation carries a smaller identifier, it is
    fewer than `k` positions away. -/
theorem displacement (T : Trace) {i j : Nat} (hinv : T.A j < T.A i) : j < i + T.k := by
  cases Nat.lt_or_ge j (i + T.k) with
  | inl h => exact h
  | inr h =>
    exfalso
    have h' : i + T.k ≤ j := h
    have := T.no_inversion h'
    omega

end Trace

/-! ## 4.  The local (64-bit) value and its sequencer

`guid.L` drops the ⟨l⟩ field, so a local value is

```
     3            47            14
   |---|--------------------|--------|
    ⟨d⟩         ⟨x⟩           ⟨s⟩
```

and its order is decided by the pair ⟨x,s⟩ alone.  `sequence` (sequence.go)
allocates that pair as a single word `V = x·2^14 + s` which is only ever
incremented or raised, never lowered.  The theorems below say that this is
exactly what local ordering requires, and that the scheme it replaces --
⟨x⟩ read from the clock, ⟨s⟩ taken as `n % 2^14` from an independent counter --
is not merely fragile but demonstrably wrong.
-/

/-- The 64-bit local layout ⟨d⟩‖⟨x⟩‖⟨s⟩ as a natural number. -/
def packL (d x s : Nat) : Nat := (d * 2 ^ 47 + x) * 2 ^ 14 + s

/-- Splitting the packed word `V` into ⟨x,s⟩ and laying the two out in the
    local schema reproduces `V` itself, under a constant drift prefix. -/
theorem packL_split (d v : Nat) :
    packL d (v / 2 ^ 14) (v % 2 ^ 14) = d * 2 ^ 47 * 2 ^ 14 + v := by
  have hdm : v / 2 ^ 14 * 2 ^ 14 + v % 2 ^ 14 = v := by
    rw [Nat.mul_comm]
    exact Nat.div_add_mod v (2 ^ 14)
  show (d * 2 ^ 47 + v / 2 ^ 14) * 2 ^ 14 + v % 2 ^ 14 = d * 2 ^ 47 * 2 ^ 14 + v
  rw [Nat.add_mul, Nat.add_assoc, hdm]

/-- **A strictly increasing `V` yields strictly increasing local values.**
    This is why `sequence` keeps ⟨t⟩ and ⟨s⟩ as one word. -/
theorem packL_lt_of_lt {d v v' : Nat} (h : v < v') :
    packL d (v / 2 ^ 14) (v % 2 ^ 14) < packL d (v' / 2 ^ 14) (v' % 2 ^ 14) := by
  rw [packL_split, packL_split]
  exact Nat.add_lt_add_left h _

/-- **The scheme this replaces is not monotone.**  With ⟨x⟩ read from the clock
    and ⟨s⟩ taken as `n % 2^14` from an independent counter, the allocation
    numbered `n = 16384` sorts strictly *before* the allocation numbered
    `n = 16383` whenever the two share a tick ⟨x⟩ -- although only two
    allocations occur in that tick, four orders of magnitude under the 2^14
    budget.  What breaks the order is crossing a multiple of `2^14`, not the
    number of allocations between the two. -/
theorem wrap_inverts (d x : Nat) :
    packL d x (16384 % 16384) < packL d x (16383 % 16384) := by
  show (d * 2 ^ 47 + x) * 2 ^ 14 + 16384 % 16384
     < (d * 2 ^ 47 + x) * 2 ^ 14 + 16383 % 16384
  omega

/-- An execution of the local allocator.  `V i` is the packed ⟨x,s⟩ word handed
    to the `i`-th allocation, indexed by the order of the atomic add that
    produced it.  `mono` holds by construction of `sequence.next`: the word is
    only ever incremented or raised to the clock, so each add returns a value
    strictly greater than every value returned before it.  Nothing is assumed
    about the clock -- not even that it moves forwards. -/
structure Local where
  d : Nat
  V : Nat → Nat
  mono : ∀ i j, i < j → V i < V j

namespace Local

/-- The local value produced by the `i`-th allocation. -/
def A (L : Local) (i : Nat) : Nat :=
  packL L.d (L.V i / 2 ^ 14) (L.V i % 2 ^ 14)

/-- **Theorem 1.**  Local values sort exactly in allocation order.  Since the
    atomic add lies inside the operation that returns the value, this order
    extends the real-time order of any execution, and is a linearization of the
    strictly-increasing-generator specification. -/
theorem sorted (L : Local) {i j : Nat} (h : i < j) : L.A i < L.A j :=
  packL_lt_of_lt (L.mono i j h)

/-- Local values are pairwise distinct. -/
theorem distinct (L : Local) {i j : Nat} (h : i ≠ j) : L.A i ≠ L.A j := by
  cases Nat.lt_or_ge i j with
  | inl hlt => exact Nat.ne_of_lt (L.sorted hlt)
  | inr hge =>
    have hlt : j < i := by omega
    exact (Nat.ne_of_lt (L.sorted hlt)).symm

end Local

/-! ## 5.  Non-vacuity

A structure with contradictory fields would make every theorem above trivially
true, so we exhibit an execution satisfying all of them: drift `D = 21`
(the library default, `Δ = 2^38 ns ≈ 274.9 s`), perfectly synchronized clocks,
one allocation per drift window, hence `k = 1`.
-/

def sane : Trace where
  D := 21
  hD := by decide
  delta := 2 ^ 38
  hdelta := rfl
  eps := 0
  k := 1
  tau := fun i => i * 2 ^ 38
  clk := fun i => i * 2 ^ 38
  node := fun _ => 0
  seq := fun _ => 0
  hnode := fun _ => by show 0 < 2 ^ 32; decide
  hseq := fun _ => by show 0 < 2 ^ 14; decide
  mono_tau := by
    intro i j h
    show i * 2 ^ 38 ≤ j * 2 ^ 38
    omega
  skew_hi := by
    intro i
    show i * 2 ^ 38 ≤ i * 2 ^ 38 + 0
    omega
  skew_lo := by
    intro i
    show i * 2 ^ 38 ≤ i * 2 ^ 38 + 0
    omega
  rate := by
    intro i j h
    show i * 2 ^ 38 + (2 ^ 38 + 2 * 0) ≤ j * 2 ^ 38
    omega

example : sane.A (sane.k + 1 - sane.k) ≤ sane.A (sane.k + 1) :=
  (sane.k_ordered (Nat.lt_succ_self _)).left

-- No `sorryAx`, no `Classical.choice`: every theorem below is fully constructed.
#print axioms Guid.Trace.no_inversion_of_time
#print axioms Guid.Trace.no_inversion
#print axioms Guid.Trace.k_ordered
#print axioms Guid.Trace.inversion_window
#print axioms Guid.Trace.displacement
#print axioms Guid.pack_lt_of_epoch_lt
#print axioms Guid.pack_le_of_suffix_le
#print axioms Guid.sane
#print axioms Guid.packL_lt_of_lt
#print axioms Guid.wrap_inverts
#print axioms Guid.Local.sorted
#print axioms Guid.Local.distinct

end Guid
