package crypto

import "errors"

// ErrIndexOutOfRange is returned when a proof is requested for al leaf that does not exist.
var ErrIndexOutOfRange = errors.New("leaf index out of range")

//Domain-separation prefixes (RFC 6962,as used by cometBFT): a leaf hash can never
// be confused with an inner-noode hash.

const (
	leafPrefix  = 0x00
	innerPrefix = 0x01
)

// Proof shows that one leaf is part of a Merkle tree with a given root.
type Proof struct {
	Index int    //position of the leaf
	Total int    //number of leaves in the tree
	Aunts []Hash // sibling hashes on the path from the leaf to the root, bottom first.

}

func leafHash(leaf []byte) Hash {
	return Sum(append([]byte{leafPrefix}, leaf...))
}

func innerHash(left, right Hash) Hash {
	b := make([]byte, 0, 1+2*Size)
	b = append(b, innerPrefix)
	b = append(b, left[:]...)
	b = append(b, right[:]...)
	return Sum(b)
}

// Split returns the largest power of two strictly less than n(n>=2).

func split(n int) int {
	k := 1
	for k*2 < n {
		k *= 2
	}
	return k
}

//MerkleRoot returns the root hash of the tree built from leaves.
//An empty list has the root Sum(nil).

func MerkleRoot(leaves [][]byte) Hash {
	switch len(leaves) {
	case 0:
		return Sum(nil)
	case 1:
		return leafHash(leaves[0])
	}
	k := split(len(leaves))
	return innerHash(MerkleRoot(leaves[:k]), MerkleRoot(leaves[k:]))
}

// MerkleProof returns a proof that leaves[index] is in the tree.
func MerkleProof(leaves [][]byte, index int) (Proof, error) {
	if index < 0 || index >= len(leaves) {
		return Proof{}, ErrIndexOutOfRange
	}
	return Proof{Index: index, Total: len(leaves), Aunts: aunts(leaves, index)}, nil
}

// Aunts collects the sibling hashes from the leaf upto the root(bottom first).
func aunts(leaves [][]byte, index int) []Hash {
	if len(leaves) <= 1 {
		return nil
	}
	k := split(len(leaves))
	if index < k {
		return append(aunts(leaves[:k], index), MerkleRoot(leaves[k:]))
	}
	return append(aunts(leaves[k:], index-k), MerkleRoot(leaves[:k]))
}

// verify reports whether leaf, at p.Index is part of the tree with this root.
func (p Proof) Verify(root Hash, leaf []byte) bool {
	if p.Index < 0 || p.Index >= p.Total {
		return false
	}
	h, ok := hashFromAunts(p.Index, p.Total, leafHash(leaf), p.Aunts)
	return ok && h == root
}

// hashFrom Aunts rebuilds the root from a leaf hash and its aunts.
func hashFromAunts(index, total int, leaf Hash, aunts []Hash) (Hash, bool) {
	if total == 1 {
		return leaf, len(aunts) == 0
	}
	if len(aunts) == 0 {
		return Hash{}, false
	}
	k := split(total)
	last, rest := aunts[len(aunts)-1], aunts[:len(aunts)-1]
	if index < k {
		left, ok := hashFromAunts(index, k, leaf, rest)
		return innerHash(left, last), ok
	}
	right, ok := hashFromAunts(index-k, total-k, leaf, rest)
	return innerHash(last, right), ok
}
