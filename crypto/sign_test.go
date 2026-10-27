package crypto

import (
	"bytes"
	"errors"
	"testing"
)

// Seedkey returns a detetrminitstic key whose ssed is 32 copies of bytes b.
func seededKey(t *testing.T, b byte) PrivateKey {
	t.Helper()
	k, err := NewKeyFromSeed(bytes.Repeat([]byte{b}, SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

//TestSignVerify: a valid signature verifies. A chnaged message, wrong key or flipped bit is rejected.

func TestSignVerify(t *testing.T) {
	k := seededKey(t, 1)
	msg := []byte("prevote height=5 round=0 block=abc")

	sig := k.Sign(msg)

	//Real Signature Verifies.
	if !Verify(k.Public(), msg, sig) {
		t.Fatalf("valid signature rejected")
	}
	// A changed message is rjected(integrity)
	if Verify(k.Public(), []byte("prevote height=5 round=0 block=EVIL"), sig) {
		t.Fatalf("tampered mesaage accepted")
	}
	//Another Validator's key is rjeected(authenticity)
	if Verify(seededKey(t, 2).Public(), msg, sig) {
		t.Fatalf("Wrong public key accepted")
	}
	// A flipped signature bit is rejected.
	bad := sig  //arrays are COPIED on assignment, so bad is independent of sig
	bad[0] ^= 1 //flip the lowest bit of byte 0
	if Verify(k.Public(), msg, bad) {
		t.Fatalf("tampered signature accepted")
	}
}

func TestDeterminism(t *testing.T) {
	a, b, c := seededKey(t, 7), seededKey(t, 7), seededKey(t, 8)
	if a.Public() != b.Public() {
		t.Fatal("same seed,different keys")
	}
	if a.Public() == c.Public() {
		t.Fatal("different seed, same key")
	}
	msg := []byte("x")
	if a.Sign(msg) != b.Sign(msg) {
		t.Fatal("Ed25519 signature should be deterministic")
	}
}

func TestBadSeed(t *testing.T) {
	seed := []byte("short")
	_, err := NewKeyFromSeed(seed)
	if !errors.Is(err, ErrBadSeed) {
		t.Fatalf("err=%v,want %v", err, ErrBadSeed)
	}

}

func TestGenerateKeyRandom(t *testing.T) {
	a, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey:%v", err)
	}
	b, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey:%v", err)
	}
	if a.Public() == b.Public() {
		t.Fatalf("two random keys are equal")
	}
}

func TestAddress(t *testing.T) {
	k := seededKey(t, 7)
	pub := k.Public()
	addr := pub.Address()

	h := Sum(pub[:])
	if !bytes.Equal(addr[:], h[:AddressSize]) {
		t.Fatalf("address %x != first 20 bytes of sha256(pub) %x", addr, h[:20])
	}
	other := seededKey(t, 8).Public().Address()
	if addr == other {
		t.Fatal("different keys produced the same address")
	}
	t.Logf("address = %s", addr)

}

func TestDomainSeparation(t *testing.T) {
	k := seededKey(t, 1)
	prevote := HashFields([]byte("prevote"), Uint64Bytes(5), []byte("blockA"))
	precommit := HashFields([]byte("precommit"), Uint64Bytes(5), []byte("blockA"))
	sig := k.Sign(prevote[:])
	if Verify(k.Public(), precommit[:], sig) {
		t.Fatal("prevote signature was accepted as a precommit")
	}
}
