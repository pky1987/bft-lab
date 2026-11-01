package types

import (
	"testing"
)

func TestProposalSignVerify(t *testing.T) {
	const chain = "bft-lab-1"
	k := key(t, 1)
	p := Proposal{
		Height: 1,
		Round:  0,
		Block:  NewBlock(header(), [][]byte{[]byte("tx")}),
	}
	p.Sign(chain, k)

	if !p.Verify(chain, k.Public()) {
		t.Fatal("proposal signature failed to verify")
	}
	// Another chain.
	if p.Verify("another-chain", k.Public()) {
		t.Fatal("proposal verified with another chain")
	}

	// Wrong key.
	wrongKey := key(t, 2)
	if p.Verify(chain, wrongKey.Public()) {
		t.Fatal("proposal verified with wrong key")
	}

	// Changed round, without re-signing.
	changedRound := p
	changedRound.Round = 1

	if changedRound.Verify(chain, k.Public()) {
		t.Fatal("proposal verified after round changed")
	}

	// Changed height, without re-signing.
	changedHeight := p
	changedHeight.Height = 2

	if changedHeight.Verify(chain, k.Public()) {
		t.Fatal("proposal verified after height changed")
	}

	// Changed block header height, without re-signing.
	changedBlock := p
	changedBlock.Block.Header.Height++

	if changedBlock.Verify(chain, k.Public()) {
		t.Fatal("proposal verified after block header height changed")
	}

}

func TestProposalAndVoteSignaturesDiffer(t *testing.T) {
	const chain = "bft-lab-1"

	k := key(t, 1)

	p := Proposal{
		Height: 1,
		Round:  0,
		Block:  NewBlock(header(), [][]byte{[]byte("tx")}),
	}

	p.Sign(chain, k)

	v := Vote{
		Type:      Prevote,
		Height:    1,
		BlockHash: p.Block.Hash(),
		Validator: k.Public().Address(),
	}

	// Copy proposal signature into vote without signing the vote.
	v.Signature = p.Signature

	if v.Verify(chain, k.Public()) {
		t.Fatal("proposal signature incorrectly verified as vote signature")
	}
}
