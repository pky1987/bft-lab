package types

import (
	"bytes"
	"errors"
	"github.com/pky1987/bft-lab/crypto"
	"math"
	"testing"
)

// key returns a deterministic private key from a one-byte seed.
func key(t *testing.T, b byte) crypto.PrivateKey {
	t.Helper()
	k, err := crypto.NewKeyFromSeed(bytes.Repeat([]byte{b}, crypto.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// val returns a validator with seed b and power p.
func val(t *testing.T, b byte, p uint64) Validator {
	return NewValidator(key(t, b).Public(), p)
}

// header returns a minimal valid header.
func header() Header {
	return Header{ChainID: "bft-lab-1", Height: 1}
}

func TestValidatorSetSortedDeterministic(t *testing.T) {
	a, err := NewValidatorSet([]Validator{val(t, 1, 10), val(t, 2, 20), val(t, 3, 30)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewValidatorSet([]Validator{val(t, 3, 30), val(t, 1, 10), val(t, 2, 20)})
	if err != nil {
		t.Fatal(err)
	}
	// Claim 1: order of input doesn't matter
	if a.Hash() != b.Hash() {
		t.Fatal("same validators in different order gave different set hash")
	}
	// Claim 2: stored sorted by address
	vs := a.Validators()
	for i := 1; i < len(vs); i++ {
		if bytes.Compare(vs[i-1].Address[:], vs[i].Address[:]) >= 0 {
			t.Fatalf("validators %d and %d are not in in address order", i-1, i)
		}
	}
	// Claim 3: totals
	expectedTotal := uint64(60)
	expectedSize := 3
	if a.TotalPower() != expectedTotal || a.Size() != expectedSize {
		t.Fatalf("total=%d size=%d, want=%d and want=%d ", a.TotalPower(), a.Size(), expectedTotal, expectedSize)
	}
}

func TestValidatorSetErrors(t *testing.T) {
	cases := []struct {
		name string
		vals []Validator
		want error
	}{
		{
			name: "empty",
			vals: nil,
			want: ErrEmptyValidatorSet,
		},
		{
			name: "zero error",
			vals: []Validator{val(t, 1, 0)},
			want: ErrZeroPower,
		},
		{
			name: "dupliacte validator",
			vals: []Validator{val(t, 1, 10), val(t, 1, 20)},
			want: ErrDuplicateValidator,
		},
		{
			name: "overflow",
			vals: []Validator{val(t, 1, math.MaxUint64), val(t, 2, 1)},
			want: ErrPowerOverflow,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewValidatorSet(c.vals)
			if !errors.Is(err, c.want) {
				t.Fatalf("%s: got error %v, want %v", c.name, err, c.want)
			}
		})
	}

}

func TestValidatorSetLookupAndHash(t *testing.T) {
	s, err := NewValidatorSet([]Validator{val(t, 1, 10), val(t, 2, 20), val(t, 3, 30)})
	if err != nil {
		t.Fatal(err)
	}

	//Claim 1: lookup an existing validator & then on unknown validator.

	address := val(t, 2, 20).Address
	got, ok := s.GetByAddress(address)
	if !ok || got.Power != 20 {
		t.Fatalf("lookup of seed 2: ok=%v power=%d, want true and 20", ok, got.Power)
	}

	address1 := val(t, 9, 20).Address
	_, ok = s.GetByAddress(address1)
	if ok {
		t.Fatal("found a validator that is not in the set")
	}

	//Claim 2:change on validator's power
	changed, err := NewValidatorSet([]Validator{val(t, 1, 10), val(t, 2, 21), val(t, 3, 30)})
	if err != nil {
		t.Fatal(err)
	}
	if s.Hash() == changed.Hash() {
		t.Fatal("changing one validator's power did not change the set hash")
	}

	//claim3: mutate the slices returned by VAlidators.
	before := s.Hash()
	vs := s.Validators()
	vs[0].Power = 60
	if s.Hash() != before {
		t.Fatal("changing the returned slice must NOT change the set: internal slice leaked")
	}

}

func TestBlockValidateBasic(t *testing.T) {
	b := NewBlock(header(), [][]byte{[]byte("tx1"), []byte("tx2")})
	if err := b.ValidateBasic(); err != nil {
		t.Fatal(err)
	}
	// 1. tampered txs (header still has the OLD TxRoot)
	bad := b
	bad.Txs = [][]byte{[]byte("tx1"), []byte("tx3")}
	if !errors.Is(bad.ValidateBasic(), ErrBadTxRoot) {
		t.Fatal("tampered txs were accepted: TxRoot not checked")
	}
	//2. height 0
	z := NewBlock(Header{ChainID: "bft-lab-1"}, nil)
	if !errors.Is(z.ValidateBasic(), ErrZeroHeight) {
		t.Fatal("height 0 block was accepted")
	}
	//3.Empty chainID
	e := NewBlock(Header{Height: 2}, nil)
	if !errors.Is(e.ValidateBasic(), ErrEmptyChainID) {
		t.Fatal("block with empty chain ID was accepted")
	}
}

func TestHeaderHashEveryField(t *testing.T) {
	base := header()
	h0 := base.Hash()

	cases := []struct {
		name   string
		mutate func(*Header)
	}{
		{
			name: "chain ID", mutate: func(h *Header) { h.ChainID = "other" },
		},
		{
			name: "height", mutate: func(h *Header) { h.Height = 2 },
		},
		{
			name: "prev hash", mutate: func(h *Header) { h.PrevHash[0] = 1 },
		},
		{
			name: "tx root", mutate: func(h *Header) { h.TxRoot[0] = 1 },
		},
		{
			name: "validatorHash", mutate: func(h *Header) { h.ValidatorHash[0] = 1 },
		},
		{
			name: "proposer", mutate: func(h *Header) { h.Proposer[0] = 1 },
		},
	}
	for _, c := range cases {
		h := base
		c.mutate(&h)
		if h.Hash() == h0 {
			t.Fatalf("%s is not covered by header hash", c.name)
		}
	}
}

func TestBlockChainLink(t *testing.T) {
	b1 := NewBlock(header(), nil)
	h2 := header()

	h2.Height = 2
	h2.PrevHash = b1.Hash()
	b2 := NewBlock(h2, nil)

	//claim1: b2 links to b1
	if b2.Header.PrevHash != b1.Hash() {
		t.Fatal("b2.PrevHash does not point to b1")
	}
	//claim2:they are different block:
	if b1.Hash() == b2.Hash() {
		t.Fatal("b1 and b2 have same hash.")
	}
	//claim3: tampering with b1 breaks the links
	tampered := NewBlock(header(), [][]byte{[]byte("EVIL")})
	if b2.Header.PrevHash == tampered.Hash() {
		t.Fatal("tampered b1 still matches b2.PrevHash: link not broken")
	}
}

func TestVoteSignVerify(t *testing.T) {
	const chain = "bft-lab-1"
	k := key(t, 1)
	v := Vote{
		Type:      Prevote,
		Height:    5,
		Round:     0,
		BlockHash: crypto.Sum([]byte("B")),
		Validator: k.Public().Address(),
	}
	v.Sign(chain, k)
	//valid vote verfies
	if !v.Verify(chain, k.Public()) {
		t.Fatal("Valid vote rejected")
	}
	//cross-chain-replay
	if v.Verify("other-chain", k.Public()) {
		t.Fatal("vote replayed on another chain")
	}
	//round copy,chnage,don't re-sign
	w := v
	w.Round = 1
	if w.Verify(chain, k.Public()) {
		t.Fatal("vote replayed in another round.")
	}
	// prevote reused as precommit
	x := v
	x.Type = Precommit
	if x.Verify(chain, k.Public()) {
		t.Fatal("prevote accepted as precommit")
	}
	// swap the block
	y := v
	y.BlockHash = crypto.Sum([]byte("C"))
	if y.Verify(chain, k.Public()) {
		t.Fatal("signature accepted for a different block")
	}
	// wrong key
	if v.Verify(chain, key(t, 2).Public()) {
		t.Fatal("vote verified with the wrong key")
	}
	// forged validator field
	z := v
	z.Validator = key(t, 2).Public().Address()
	if z.Verify(chain, k.Public()) {
		t.Fatal("vote accepted with a forged validator address")
	}
	// nil votes
	if v.IsNil() {
		t.Fatal("vote for a block reported as nil")
	}
	n := Vote{Type: Prevote, Height: 5, Validator: k.Public().Address()} // no BlockHash
	n.Sign(chain, k)
	if !n.IsNil() || !n.Verify(chain, k.Public()) {
		t.Fatal("nil vote must be IsNil and still verify")
	}
}
