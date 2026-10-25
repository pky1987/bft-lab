# `crypto`: hashing (and later signatures and Merkle trees)

> **One sentence:** `crypto` turns any data into a short, fixed-size **fingerprint** (a hash) that
> changes completely if even one bit of the data changes. That's what lets a blockchain detect tampering.

Lesson 8 = hashing (this page). Lesson 9 = Ed25519 signatures. Lesson 10 = Merkle trees. Each gets its own section here.

---

## 1. Why hashing exists in a blockchain

Validators need to agree on **huge** amounts of data (every block, every transaction). They can't send
all of it to each other every time they vote. Instead they vote on a **32-byte fingerprint** of the data:

```
Block (could be megabytes)  ──SHA-256──►  ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
                                          (always 32 bytes = 256 bits = 64 hex characters)
```

If two validators have the same hash, they have the same block. If one byte differs, the hashes differ
completely. A vote in CometBFT is literally *"I vote for the block with hash X"*.

Hashes show up everywhere in this plan:

| Where | Hash used for |
|---|---|
| block header | `PrevHash` links each block to the one before it (the **chain** in blockchain) |
| votes (Lesson 9) | you sign the **hash** of a vote, not the whole vote |
| Merkle tree (Lesson 10) | proving one transaction is inside a block without sending the whole block |
| Cosmos app hash (P2) | a hash of the entire application state, agreed on every block |
| IBC (P3) | proofs that a packet was committed on another chain |
| ZK (P5) | commitments and nullifiers are hashes |

---

## 2. Theory: what makes a cryptographic hash function

A hash function `H` takes **any** input and returns a **fixed-size** output. A *cryptographic* one, like
SHA-256, also has these properties:

| Property | Meaning | Why the blockchain needs it |
|---|---|---|
| **Deterministic** | same input → same hash, on every machine, every time | all validators must compute the same block hash |
| **Fixed size** | output is always 32 bytes | votes and headers stay small |
| **Avalanche effect** | flip 1 input bit → about **half** the output bits change | a tiny edit is impossible to hide |
| **Preimage resistance** | given a hash, you can't find an input that produces it | you can't forge data to match a hash |
| **Collision resistance** | you can't find **two different** inputs with the same hash | nobody can swap a block for a different one with the same hash |

**Measured in our prototype:** changing `"vote:yes"` to `"vote:yez"` (one letter) changed **140 of the 256
output bits**, close to the expected half.

**Known test vector:** `SHA-256("abc") = ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad`.
Every correct SHA-256 implementation in the world gives this answer, so a test that checks it proves we're
calling the real algorithm correctly.

> **Never write your own hash function.** Use the standard library (`crypto/sha256`). Our package only
> **wraps** it with types and helpers.

---

## 3. Theory: hash chains, or why it's called a blockchain

Each block stores the hash of the previous block, and its own hash covers that `PrevHash`:

```
 ┌────────────┐      ┌────────────┐      ┌────────────┐      ┌────────────┐
 │ Block 0    │      │ Block 1    │      │ Block 2    │      │ Block 3    │
 │ prev: 0000 │◄─────│ prev: H0   │◄─────│ prev: H1   │◄─────│ prev: H2   │
 │ data: b0   │      │ data: b1   │      │ data: b2   │      │ data: b3   │
 └────────────┘      └────────────┘      └────────────┘      └────────────┘
   H0 = H(0000,b0)     H1 = H(H0,b1)       H2 = H(H1,b2)       H3 = H(H2,b3)
```

**Change block 2's data → H2 changes → block 3's `prev` no longer matches → H3 changes → every later
hash changes.** To tamper with one old block, an attacker would have to rewrite **every block after it**,
and convince the validators to accept that. That's why the latest hash is a fingerprint of the
**entire history**.

Measured in the prototype: with `b2` replaced by `"EVIL"`, blocks 0–1 kept their hashes and blocks **2, 3 and 4
all changed**.

The first block has no previous block, so its `prev` is the **zero hash** (32 zero bytes). That's what `IsZero` checks.

---

## 4. Theory: canonical encoding, the bug most people miss

To hash a struct (a header with height, prev hash and data), you first have to turn it into bytes. The
obvious approach is to **just concatenate the fields**, and it's **broken**:

```
fields ("ab", "c")   → concatenated: "abc"  → H("abc")
fields ("a", "bc")   → concatenated: "abc"  → H("abc")   ← SAME HASH for different data 💥
```

Two different headers would have the same hash: a real collision, created by **us**, not by SHA-256.
An attacker can exploit this to make validators agree on one hash while meaning different data.

**Fix: length-prefixing.** Write each field's **length** before the field:

```
("ab", "c")  → [len=2]"ab" [len=1]"c"
("a", "bc")  → [len=1]"a"  [len=2]"bc"     ← different bytes → different hashes ✅
```

Verified in the prototype: removing the length prefix makes `TestHashFieldsUnambiguous` fail with a real collision.

### Endianness: numbers must be encoded identically everywhere

A `uint64` like `1` is 8 bytes in memory, but **which byte comes first** depends on the CPU. To make
every validator produce the same bytes, we always use **big-endian** (most significant byte first):

```
Uint64Bytes(1) = 00 00 00 00 00 00 00 01      (big-endian, verified)
```

This is the same determinism rule as "no floats, no map iteration order" in `ledger`: **every input to a
hash must be encoded byte-for-byte the same on every machine.** Cosmos uses protobuf with deterministic
encoding for exactly this reason.

---

## 5. Types and functions

```go
const Size = sha256.Size          // 32
type Hash [Size]byte              // a fixed-size ARRAY, not a slice

func Sum(data []byte) Hash                 // SHA-256 of data
func (h Hash) String() string              // 64 hex characters
func (h Hash) IsZero() bool                // all 32 bytes are zero?
func Uint64Bytes(n uint64) []byte          // 8 big-endian bytes
func HashFields(fields ...[]byte) Hash     // unambiguous hash of several fields
```

| Go concept | Where | Meaning |
|---|---|---|
| **array** `[32]byte` vs **slice** `[]byte` | `type Hash [Size]byte` | an array has a **fixed** size that's part of its type. Arrays can be compared with `==` and used as map keys, and slices can't. Perfect for hashes |
| `h[:]` | `String`, `copy(out[:], ...)` | turns an array into a slice pointing at the same bytes |
| **variadic** `...[]byte` | `HashFields` | accepts any number of arguments: `HashFields(a, b, c)` |
| `sha256.New()` + `Write` + `Sum(nil)` | `HashFields` | a **streaming** hasher: feed it pieces, then get the final hash |
| `hex.EncodeToString` | `String` | bytes → readable hex text |
| `binary.BigEndian.PutUint64` | `Uint64Bytes` | number → 8 bytes in a fixed order |
| method on a **value** receiver `(h Hash)` | `String`, `IsZero` | the hash is small and never modified, so a copy is fine (unlike `*Ledger`) |

---

## 6. Properties and the tests that will prove them

| Property | Test |
|---|---|
| we compute real SHA-256 (matches the known `"abc"` vector) | `TestSumKnownVector` |
| one letter changed → at least 64 of 256 bits change (measured: 140) | `TestAvalanche` |
| `("ab","c")` ≠ `("a","bc")`, and field order matters | `TestHashFieldsUnambiguous` |
| zero hash detection, `Sum(nil)` isn't zero, big-endian encoding | `TestIsZero` |
| editing one block changes that block's hash and every later one | `TestHashChainTamper` |

**Break experiment (verified):** delete the length prefix in `HashFields`, and `TestHashFieldsUnambiguous` fails with "collision".

---

## 7. How this maps to CometBFT / Cosmos

| Here | CometBFT / Cosmos |
|---|---|
| `crypto.Hash`, `Sum` | CometBFT `crypto/tmhash` (SHA-256) |
| hash chain via `prev` | `Header.LastBlockID` points at the previous block's hash |
| `HashFields` (unambiguous encoding) | headers are hashed via a Merkle tree of **protobuf-encoded** fields: deterministic, length-delimited |
| big-endian integers | protobuf's fixed encoding rules |

---

## 8. Interview questions

1. What properties must a cryptographic hash function have, and why does a blockchain need each one?
2. Why does changing an old block invalidate every block after it?
3. Why is concatenating fields before hashing dangerous? How do you fix it?
4. Why must hash inputs be encoded deterministically? Give two sources of non-determinism.
5. Why use an array `[32]byte` instead of a slice `[]byte` for a hash type in Go?

---

## 9. In my own words (Prakash, fill this in)

- A hash is:
- The avalanche effect means:
- A hash chain detects tampering because:
- Length-prefixing prevents:
