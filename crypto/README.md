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

### Results (Lesson 8 ✅, committed `998df6f` on 2026-10-25)

- 5 tests, 100% coverage, lint 0
- Avalanche, `"vote:yes"` vs `"vote:yed"`: **141 of 256** bits differ (measured on the hash **bytes**)

---

---

# Lesson 9: Ed25519 digital signatures (`sign.go`)

## 9.1 Why signatures exist in consensus

A hash proves **what** the data is. It says nothing about **who** sent it. In consensus, *who* matters:

> "Validator X votes for block H" only counts if **X really said it**. Otherwise an attacker could flood the
> network with fake votes "from" honest validators and fake a 2/3 majority.

A **digital signature** solves this:

```
      Validator X (has a PRIVATE key, kept secret)            Everyone else (knows X's PUBLIC key)
      ────────────────────────────────────────────            ───────────────────────────────────
      msg = "prevote h=5 r=0 block=H"
      sig = Sign(privateKey, msg)      ──── msg + sig ────►   Verify(publicKey, msg, sig)
                                                                 → true:  X really signed exactly this msg
                                                                 → false: forged, or msg/sig changed in transit
```

| Guarantee | Meaning |
|---|---|
| **Authenticity** | only the holder of the private key can produce a valid signature |
| **Integrity** | changing even one bit of the message (or of the signature) makes `Verify` fail |
| **Non-repudiation** | X can't later deny signing. This is how **double-voting** is proven and punished (evidence, topic 22) |

## 9.2 Theory: public-key cryptography (the intuition)

- A key pair is generated together: the **private key** (32-byte seed, secret) and the **public key** (32 bytes, shared freely)
- **Sign** needs the private key. **Verify** needs only the public key
- It's computationally infeasible to compute the private key from the public key, or to make a valid signature without it

**Analogy:** a wax seal. Only you own the seal stamp (the private key), but anyone who knows what your seal
looks like (the public key) can check that a letter was sealed by you, and that the seal hasn't been broken.

**Why Ed25519?** It's the signature scheme CometBFT validators use by default for consensus votes:

| Property | Ed25519 |
|---|---|
| public key / signature size | 32 bytes / 64 bytes |
| **deterministic** signatures | same key + same message → **same** signature. No random number needed at signing time |
| speed (measured on this machine, rough) | sign ≈ **19 µs**, verify ≈ **37 µs** |

**Why "deterministic" matters:** older schemes like ECDSA need a fresh random number for **every** signature.
If that number is ever **reused or predictable**, the private key can be computed from two signatures. That
has caused real key thefts, including the famous PlayStation 3 signing-key break. Ed25519 derives that number from the key and the
message, so this class of bug can't happen.

## 9.3 Theory: addresses

A public key is 32 bytes. Like CometBFT, we derive a shorter **address**: the first 20 bytes of SHA-256(public key).

```
private key ──► public key (32 B) ──SHA-256──► first 20 bytes ──► address (40 hex chars)
```
This is what the `ledger` README meant by *"on a real chain, an address is derived from a public key"*.
Measured: seed `0x01…` → address `34750f98bd59fcfc946da45aaabe933be154a4b5`.

## 9.4 Theory: what a signature does NOT protect against

| Attack | Signature alone stops it? | Real defence |
|---|---|---|
| forged message | ✅ yes | – |
| modified message | ✅ yes | – |
| **replay**: re-sending an old, valid signed message | ❌ **no**, it's still a valid signature | put **height, round, chain-id** inside the signed message so it's only valid once (topic 11), or use nonces (P2/P6) |
| **cross-context reuse**: a prevote signature accepted as a precommit | ❌ no, if both sign the same bytes | **domain separation**: include the message **type** in the signed bytes (`HashFields("prevote", …)`) |
| stolen private key | ❌ no | key management. In P2: never commit keys, and use `.gitignore`. Production uses HSMs and remote signers |

Domain separation is verified in the prototype: a signature over `HashFields("prevote", 5, blockA)` **does not**
verify for `HashFields("precommit", 5, blockA)`.

## 9.5 Theory: seeded keys for the simulator

Real nodes need **random** keys (`GenerateKey`, using `crypto/rand`). But the simulator (topic 19) must be
**reproducible**: the same seed must give the same validators every run. `NewKeyFromSeed` derives a key from a
fixed 32-byte seed, which is the same idea as the seeded RNG in `LossyTransport`.

> ⚠️ **Never use a predictable seed for a real validator.** Seeded keys are for tests and simulations only.
> And never use `math/rand` for keys: only `crypto/rand` is secure.

## 9.6 Types and functions

```go
type PublicKey  [32]byte
type Signature  [64]byte
type Address    [20]byte
type PrivateKey struct { key ed25519.PrivateKey }   // field is UNEXPORTED

func GenerateKey() (PrivateKey, error)               // random, for real nodes
func NewKeyFromSeed(seed []byte) (PrivateKey, error) // deterministic. ErrBadSeed unless 32 bytes
func (k PrivateKey) Public() PublicKey
func (k PrivateKey) Sign(msg []byte) Signature
func Verify(pub PublicKey, msg []byte, sig Signature) bool
func (p PublicKey) Address() Address
func (a Address) String() string
```

| Go concept | Where | Meaning |
|---|---|---|
| struct with an **unexported** field | `PrivateKey{key}` | other packages can **use** the key (`Sign`) but can't **read** the secret bytes, so it can't accidentally be printed or sent. Encapsulation as a security property, like `ledger.balances` |
| **type assertion** `x.(T)` | `k.key.Public().(ed25519.PublicKey)` | `Public()` returns an **interface** (`crypto.PublicKey`), and `.(ed25519.PublicKey)` says "I know it's really this type, give it to me as that" |
| **const block** | `const ( PublicKeySize = …; … )` | group related constants |
| returning `(value, error)` | `NewKeyFromSeed` | validate input and refuse bad seeds instead of panicking (`ed25519.NewKeyFromSeed` **panics** on a wrong length) |
| ⚠️ **unaddressable array** | `k.Public()[:]` → compile error | you can't slice an array that a function **just returned**. Store it first: `pub := k.Public(); pub[:]` |

## 9.7 Properties and the tests that will prove them

| Property | Test |
|---|---|
| a valid signature verifies. A changed message, wrong key or flipped signature bit is rejected | `TestSignVerify` |
| same seed → same key. Different seed → different key. Same key + message → same signature | `TestDeterminism` |
| a seed that isn't 32 bytes → `ErrBadSeed` (no panic) | `TestBadSeed` |
| two random keys differ | `TestGenerateKeyRandom` |
| address = SHA-256(pub)[:20], and different keys give different addresses | `TestAddress` |
| a prevote signature can't be reused as a precommit | `TestDomainSeparation` |
| for **any** message, a valid signature verifies and a 1-bit change is rejected | `FuzzSignVerify` |

**Verified mutation:** making `Verify` always return `true` is caught by `TestSignVerify` ("tampered msg
accepted") and `TestDomainSeparation`.

### Results (Lesson 9 ✅, committed `0cb6db0` on 2026-10-25)

- 6 signature tests written by Prakash (no fuzz test yet), all PASS. Package coverage **96.9%**: the only
  uncovered line is the `crypto/rand` failure branch in `GenerateKey`, which can't be triggered on purpose
- Measured: seed 7 → address `fe812c12f3ab4ce6ac5db69ac352f906cb1b11ef`
- **Bugs caught while writing the tests** (all the same family: the check didn't match the intent):
  - `TestBadSeed` first called the `seededKey` helper, which always builds a **valid** seed, so it couldn't test a bad one
  - `TestGenerateKeyRandom` used `!=` with the message "keys are equal": an inverted check that fails on correct code
  - `TestAddress` used `h[:pub]` (an index must be an integer), the **same seed** for `other` (identical keys), `Other`
    instead of `other`, `!=` instead of `==`, and `t.fatal` / `t.LogF` (method names are case-sensitive)
  - Rule that came out of it: **read each `if` together with its `t.Fatal` message. They must describe the same situation**

## 9.8 Interview questions (signatures)

1. What do digital signatures guarantee, and what do they **not** protect against?
2. Why are Ed25519 signatures deterministic, and why is that safer than ECDSA's random nonce?
3. How would an attacker replay a valid vote, and how does consensus prevent it?
4. What is domain separation? Give an example from consensus.
5. Why must `math/rand` never be used to generate keys?
6. How is a validator address derived from its public key?

## 9.9 In my own words (signatures)

- A signature proves:
- Replay is still possible because:
- Domain separation means:

---

# Lesson 10: Merkle trees and inclusion proofs (`merkle.go`)

## 10.1 Why Merkle trees exist

A block contains many transactions. Two problems:

1. **The header must commit to all of them** with one small hash, so that changing any transaction changes the block hash
2. **A light client** (a phone wallet, or another chain in IBC) wants to know *"is my transaction in block 1000?"*
   **without downloading the whole block**

Hashing all transactions in one go (`Sum(tx0 || tx1 || …)`) solves problem 1 but not problem 2: to check it, you'd
need every transaction. A **Merkle tree** solves both.

```
                         root = H(1 ‖ H01 ‖ H23)          ← goes in the block header (32 bytes)
                        /                     \
           H01 = H(1 ‖ L0 ‖ L1)        H23 = H(1 ‖ L2 ‖ L3)    ← inner nodes
             /         \                  /         \
     L0=H(0‖tx0)  L1=H(0‖tx1)    L2=H(0‖tx2)  L3=H(0‖tx3)      ← leaves
```

## 10.2 Inclusion proofs: proving one leaf with a few hashes

To prove **tx2** is in the tree, you don't send all 4 transactions. You send tx2 plus the **siblings**
along its path to the root, called the **aunts**:

```
proof for tx2:  aunts = [ L3, H01 ]          (bottom first)

verifier:  L2  = H(0 ‖ tx2)                  ← computed from the transaction itself
           H23 = H(1 ‖ L2 ‖ L3)              ← uses aunt #1 (sibling on the RIGHT)
           root' = H(1 ‖ H01 ‖ H23)          ← uses aunt #2 (sibling on the LEFT)
           root' == root from the header?  → tx2 is in the block ✅
```
The **index** tells the verifier whether each aunt sits on the left or the right. That's why a proof stores `Index` and `Total`.

**Measured proof sizes** (number of aunts × 32 bytes):

| Transactions in block | Aunts in proof | Proof size |
|---|---|---|
| 1 | 0 | 0 bytes |
| 2 | 1 | 32 bytes |
| 8 | 3 | 96 bytes |
| 100 | 7 | 224 bytes |
| 1,000 | 10 | 320 bytes |
| 1,000,000 | 20 | 640 bytes |

The proof grows with **log₂(n)**: a million times more transactions needs only 20 hashes. That's what makes
light clients and cross-chain proofs practical.

## 10.3 Unbalanced trees: the split rule

When the number of leaves isn't a power of two, where do you split? Like CometBFT (RFC 6962), we split at
**the largest power of two smaller than n**:

```
n = 3:  split at 2 →  [tx0 tx1] [tx2]          root = H(1 ‖ H(1‖L0‖L1) ‖ L2)
n = 5:  split at 4 →  [tx0..tx3] [tx4]
n = 6:  split at 4 →  [tx0..tx3] [tx4 tx5]
```
Each side is built the same way, **recursively**, until a side has one leaf.

> **Recursion** (new Go concept): a function that calls **itself** on a smaller piece of the problem.
> `MerkleRoot(leaves)` = `innerHash(MerkleRoot(left half), MerkleRoot(right half))`, stopping when there's 0 or 1 leaf
> (the **base case**). Without a base case, recursion never ends.

## 10.4 Security: the second-preimage attack, and why the 0x00 / 0x01 prefixes matter

Without prefixes, a leaf hash is `H(tx)` and an inner hash is `H(left ‖ right)`. An attacker can take the
64 bytes `L0 ‖ L1` and claim *"this is a single transaction"*. Its "leaf hash" `H(L0 ‖ L1)` is exactly the
inner node H01, so a **fake transaction gets a valid proof**.

The fix (RFC 6962, used by CometBFT): prefix leaves with `0x00` and inner nodes with `0x01`. Now a leaf hash
and an inner hash can never be equal, which is the same **domain separation** idea as `"prevote"` vs `"precommit"` in Lesson 9.

**Verified:** removing the prefixes makes `TestLeafInnerSeparation` fail with *"inner node accepted as leaf"*.

## 10.5 What a proof verifier must reject

| Attack | Defence in `Proof.Verify` |
|---|---|
| wrong transaction for this proof | the leaf hash changes, so the root doesn't match |
| right transaction, wrong root (another block) | the root doesn't match |
| wrong `Index` | aunts get combined on the wrong side, so the root doesn't match |
| a tampered aunt | the root doesn't match |
| too few / too many aunts | `hashFromAunts` returns `ok = false` |
| `Index` ≥ `Total` or negative | rejected before any hashing |

## 10.6 Types and functions

```go
var ErrIndexOutOfRange = errors.New("leaf index out of range")

type Proof struct {
    Index int    // which leaf
    Total int    // how many leaves in the tree
    Aunts []Hash // siblings from the leaf up to the root, bottom first
}

func MerkleRoot(leaves [][]byte) Hash                       // Sum(nil) for no leaves
func MerkleProof(leaves [][]byte, index int) (Proof, error)
func (p Proof) Verify(root Hash, leaf []byte) bool

// unexported helpers: leafHash, innerHash, split, aunts, hashFromAunts
```

| Go concept | Where | Meaning |
|---|---|---|
| **recursion** | `MerkleRoot`, `aunts`, `hashFromAunts` | a function calling itself on a smaller input, with a base case |
| `[][]byte` | `leaves` | a slice of byte slices: one entry per transaction |
| `switch len(x) { case 0: … case 1: … }` | `MerkleRoot` | cleaner than `if/else if` for base cases |
| `make([]byte, 0, 1+2*Size)` | `innerHash` | length 0, **capacity** 65, so the three `append`s never reallocate |
| two return values `(Hash, bool)` | `hashFromAunts` | the result, plus whether the proof had the right shape |
| `last, rest := a[len(a)-1], a[:len(a)-1]` | `hashFromAunts` | take the top aunt and the rest. Multiple assignment |

## 10.7 Properties and the tests that will prove them

| Property | Test |
|---|---|
| roots for 0, 1, 2 and 3 leaves match a hand computation | `TestMerkleRootSmall` |
| changing **any** leaf, or swapping two, changes the root | `TestMerkleRootSensitive` |
| for every tree size 1–17 and **every** index, the proof verifies (and logs proof sizes) | `TestProofAllIndices` |
| wrong leaf / root / index, a tampered aunt, a short proof, out-of-range index, an extra aunt → rejected | `TestProofRejects` |
| `MerkleProof` with index −1 or n → `ErrIndexOutOfRange` | `TestProofOutOfRange` |
| an inner node can't be passed off as a leaf (second preimage) | `TestLeafInnerSeparation` |

**Verified mutations:** removing the prefixes is caught by `TestLeafInnerSeparation`. Swapping left/right when
rebuilding the root is caught by `TestProofAllIndices` (`n=2 i=0 rejected`).

## 10.8 How this maps to CometBFT / Cosmos / IBC

| Here | Real world |
|---|---|
| `MerkleRoot(txs)` | CometBFT `Header.DataHash` (Merkle root of the block's transactions) |
| `Proof{Index, Total, Aunts}` | CometBFT `crypto/merkle.Proof`, which uses the same field names |
| 0x00 / 0x01 prefixes, split at the largest power of 2 | RFC 6962, as in CometBFT's `crypto/merkle` |
| inclusion proofs | **IBC (P3)**: chain B verifies that chain A committed a packet using Merkle proofs against A's app hash |

## 10.9 Interview questions (Merkle)

1. Why does a block header store a Merkle root instead of a plain hash of all transactions?
2. How big is an inclusion proof for n transactions, and why?
3. What is a second-preimage attack on a Merkle tree, and how do leaf/inner prefixes prevent it?
4. Why does a proof need the leaf's index?
5. How does IBC use Merkle proofs?

## 10.10 In my own words (Merkle)

- A Merkle root commits to:
- An inclusion proof contains:
- The 0x00/0x01 prefixes prevent:

---

## 7. How this maps to CometBFT / Cosmos

| Here | CometBFT / Cosmos |
|---|---|
| `crypto.Hash`, `Sum` | CometBFT `crypto/tmhash` (SHA-256) |
| `PrivateKey.Sign` / `Verify` (Ed25519) | CometBFT `crypto/ed25519`: validators sign votes and proposals |
| `PublicKey.Address()` = SHA-256(pub)[:20] | CometBFT validator address derivation |
| domain separation (`"prevote"` in the signed bytes) | CometBFT signs canonical vote bytes that include type, height, round and chain-id |
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
