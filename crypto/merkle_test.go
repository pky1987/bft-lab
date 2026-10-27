package crypto

import (
	"errors"
	"fmt"
	"testing"
)

// makeLeaves returns n fake transactions: "tx-0","tx-1",...
func makeLeaves(n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = []byte(fmt.Sprintf("tx-%d", i))
	}
	return out
}

func TestMerkleRootSmall(t *testing.T) {
	a, b, c := []byte("a"), []byte("b"), []byte("c")

	if MerkleRoot(nil) != Sum(nil) {
		t.Fatal("empty tree: root should be Sum(nil)")
	}
	if MerkleRoot([][]byte{a}) != leafHash(a) {
		t.Fatal("1 leaf: root should be leafHash(a)")
	}
	if MerkleRoot([][]byte{a, b}) != innerHash(leafHash(a), leafHash(b)) {
		t.Fatal("2 leaves: root should be innerHash(L(a),L(b))")
	}
	want3 := innerHash(innerHash(leafHash(a), leafHash(b)), leafHash(c)) // split(3)=2:[a b][c]
	if MerkleRoot([][]byte{a, b, c}) != want3 {
		t.Fatal("3 leaves: root hould be innerHash(innerHash(La,Lb),Lc)")
	}
}

func TestMerkleRootSensitive(t *testing.T) {
	original := MerkleRoot(makeLeaves(8))

	for i := 0; i < 8; i++ {
		changed := makeLeaves(8)
		changed[i] = []byte("EVIL")
		if MerkleRoot(changed) == original {
			t.Fatalf("changing leaf %d did not chnage the root", i)
		}
	}
	swapped := makeLeaves(8)
	swapped[0], swapped[1] = swapped[1], swapped[0]
	if MerkleRoot(swapped) == original {
		t.Fatal("swapping leaves 0 and 1 did not chnage the root.")
	}
}

func TestProofAllIndices(t *testing.T) {
	for n := 1; n <= 17; n++ {
		l := makeLeaves(n)
		root := MerkleRoot(l)

		for i := 0; i < n; i++ {
			p, err := MerkleProof(l, i)
			if err != nil {
				t.Fatalf("n=%d i=%d:MerkleProof error:%v", n, i, err)
			}
			if !p.Verify(root, l[i]) {
				t.Fatalf("n=%d i=%d:Valid Proof rejected:", n, i)
			}
		}
	}
	for _, n := range []int{1, 2, 8, 100, 1000} {
		p, _ := MerkleProof(makeLeaves(n), n/2)
		t.Logf("n=%-5d proof has %2d aunts )%d bytes", n, len(p.Aunts), len(p.Aunts)*Size)
	}
}

// TestProofRejects: any wrong part of a proof makes Verify return false.
func TestProofRejects(t *testing.T) {
	l := makeLeaves(8)
	root := MerkleRoot(l)
	p, err := MerkleProof(l, 3)
	if err != nil {
		t.Fatal(err)
	}

	// 1. wrong leaf
	if p.Verify(root, []byte("EVIL")) {
		t.Fatal("accepted a proof for the wrong leaf")
	}

	// 2. wrong root (a different block)
	if p.Verify(MerkleRoot(makeLeaves(9)), l[3]) {
		t.Fatal("accepted a proof against the wrong root")
	}

	// 3. wrong index
	q := p      // copy of the struct
	q.Index = 4 // Index is an int, so changing q does NOT change p
	if q.Verify(root, l[3]) {
		t.Fatal("accepted a proof with the wrong index")
	}

	// 4. tampered aunt (copy the aunts first, because a slice is SHARED, not copied)
	r := Proof{Index: p.Index, Total: p.Total, Aunts: append([]Hash(nil), p.Aunts...)}
	r.Aunts[0][0] ^= 1 // flip one bit of the first aunt
	if r.Verify(root, l[3]) {
		t.Fatal("accepted a proof with a tampered aunt")
	}

	// 5. too few aunts
	short := Proof{Index: p.Index, Total: p.Total, Aunts: p.Aunts[:2]}
	if short.Verify(root, l[3]) {
		t.Fatal("accepted a proof with missing aunts")
	}

	// 6. index outside the tree
	outside := Proof{Index: 9, Total: 8}
	if outside.Verify(root, l[3]) {
		t.Fatal("accepted a proof whose index is outside the tree")
	}

	// 7. an extra, unused aunt
	extra := Proof{Index: 0, Total: 1, Aunts: []Hash{{}}}
	if extra.Verify(MerkleRoot(l[:1]), l[0]) {
		t.Fatal("accepted a proof with an extra aunt")
	}
}

// TestProofOutOfRange: proofs for indices outside 0..n-1 return ErrIndexOutOfRange.
func TestProofOutOfRange(t *testing.T) {
	l := makeLeaves(8)
	for _, i := range []int{-1, 8} { // just below and just above the valid range
		if _, err := MerkleProof(l, i); !errors.Is(err, ErrIndexOutOfRange) {
			t.Fatalf("index %d: err = %v, want %v", i, err, ErrIndexOutOfRange)
		}
	}
}
