package crypto

import (
	"fmt"
	"math/bits"
	"testing"
)

//buildChain links blocks line a blockchain:each hash covers the previous hash.

func buildChain(data []string) []Hash {
	var prev Hash //zero hash: block 0 has no previou block.
	out := make([]Hash, len(data))
	for i, d := range data {
		out[i] = HashFields(prev[:], []byte(d))
		prev = out[i] //BREAK-POINT: A BROKEN LINK(DELETE)
	}
	return out
}

func TestHashChainTamper(t *testing.T) {
	orig := buildChain([]string{"b0", "b1", "b2", "b3", "b4"})
	tampered := buildChain([]string{"b0", "b1", "b2", "EVIL", "b3", "b4"})

	for i := 0; i < 3; i++ { //before tammpered block:unchanged
		if orig[i] != tampered[i] {
			t.Fatalf("block %d chnaged, but is BEFORE the tampered block", i)
		}
	}
	for i := 3; i < 5; i++ { //the tampered block and everything after:chnaged
		if orig[i] == tampered[i] {
			t.Fatalf("block %d did not chnages: tampering went undetected", i)
		}
	}
}

func TestSumKnownVector(t *testing.T) {
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	got := Sum([]byte("abc")).String()
	if got != want {
		t.Fatalf("Sum(\"abc\")=%s,want %s", got, want)
	}
}

func TestAvalanche(t *testing.T) {
	a := Sum([]byte("vote:yes")).String()
	b := Sum([]byte("vote:yed")).String()
	diff := 0
	for i := range a {
		diff += bits.OnesCount8(a[i] ^ b[i])
	}
	t.Logf("%d of 256 bits differ", diff)
	if diff < 64 {
		t.Fatalf("only %d of 256 bits differ, expected at least 64", diff)
	}
}

func TestHashFieldsUnambiguous(t *testing.T) {
	h1 := HashFields([]byte("ab"), []byte("c"))
	h2 := HashFields([]byte("a"), []byte("bc"))
	if h1 == h2 {
		t.Fatalf("Collision:(\"ab\",\"c\") and (\"a\",\"bc\") hash the same")
	}
	if HashFields([]byte("a"), []byte("b")) == HashFields([]byte("b"), []byte("a")) {
		t.Fatalf("oder ignored:(\"a\",\"b\") and (\"b\",\"a\") hash the same")
	}
}

func TestIsZero(t *testing.T) {
	//Empty has is zero
	if !(Hash{}).IsZero() {
		t.Fatalf("Hash{} should be zero")
	}
	//hash of input empty is not zero.
	if Sum(nil).IsZero() {
		t.Fatalf("Sum(nil) should not be zero hash")
	}
	t.Logf("Sum(nil)=%s", Sum(nil))
	//big-endian encoding of 1
	got := fmt.Sprintf("%x", Uint64Bytes(1))
	if got != "0000000000000001" {
		t.Fatalf("Uint64Bytes(1)=%s,want 0000000000000001", got)
	}
}
