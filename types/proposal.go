package types

import (
	"github.com/pky1987/bft-lab/crypto"
)

// Proposal is a proposer's signed offer of one block for a height and round.
type Proposal struct {
	Height    uint64
	Round     uint64
	Block     Block
	Signature crypto.Signature
}

//SignBytes is exactly what gets signed. The domain tag differs
//from a vote's , so a signature on one can never be replayed as the other.

func (p Proposal) SignBytes(chainID string) []byte {
	blockHash := p.Block.Hash()
	h := crypto.HashFields(
		[]byte("bft-lab/proposal"),
		[]byte(chainID),
		crypto.Uint64Bytes(p.Height),
		crypto.Uint64Bytes(p.Round),
		blockHash[:],
	)
	return h[:]
}

// Sign fills in p.Signature using key.
func (p *Proposal) Sign(chainID string, key crypto.PrivateKey) {
	p.Signature = key.Sign(p.SignBytes(chainID))
}

// Verify reports whether p was signed by pub for this chain.
func (p Proposal) Verify(chainID string, pub crypto.PublicKey) bool {
	return crypto.Verify(pub, p.SignBytes(chainID), p.Signature)
}
