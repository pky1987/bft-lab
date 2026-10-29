package types

import (
	"github.com/pky1987/bft-lab/crypto"
)

// VoteType says which consensus step a vote belongs to.
type VoteType byte

// The two vote steps of Tendermint consensus.
const (
	Prevote   VoteType = 1
	Precommit VoteType = 2
)

// Vote is one validator's signed vote for a block at a height and round.
type Vote struct {
	Type      VoteType
	Height    uint64
	Round     uint64
	BlockHash crypto.Hash //zero hash = a nil vote : saw no acceptable block
	Validator crypto.Address
	Signature crypto.Signature
}

//SignBytes is exactly what gets signed. It includes a domain tag, the chain id, the type
// height,round and block hash, so a vote can't be replayed anywhere else.

func (v Vote) SignBytes(ChainID string) []byte {
	h := crypto.HashFields(
		[]byte("bft-lab/vote"),
		[]byte(ChainID),
		[]byte{byte(v.Type)},
		crypto.Uint64Bytes(v.Height),
		crypto.Uint64Bytes(v.Round),
		v.BlockHash[:],
	)
	return h[:]
}

// Sign fills in v.Signature using key.
func (v *Vote) Sign(ChainID string, key crypto.PrivateKey) {
	v.Signature = key.Sign(v.SignBytes(ChainID))
}

// Verify reports whether v was signed by pub for this chain.
func (v Vote) Verify(ChainID string, pub crypto.PublicKey) bool {
	return pub.Address() == v.Validator && crypto.Verify(pub, v.SignBytes(ChainID), v.Signature)
}

// IsNil reports whether this is a vote for no block.
func (v Vote) IsNil() bool {
	return v.BlockHash.IsZero()
}
