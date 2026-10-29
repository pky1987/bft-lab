package types

import (
	"errors"
	"github.com/pky1987/bft-lab/crypto"
)

// Block errors
var (
	ErrEmptyChainID = errors.New("chain id is empty")
	ErrZeroHeight   = errors.New("height must be at least 1")
	ErrBadTxRoot    = errors.New("tx root does not match transaction")
)

// Header is the small,signed summary of block.

type Header struct {
	ChainID       string
	Height        uint64
	PrevHash      crypto.Hash    //hash of previous block's header(zero for height 1)
	TxRoot        crypto.Hash    // Merkle root of txs
	ValidatorHash crypto.Hash    // which validator set is responsible for this block
	Proposer      crypto.Address // who proposed it.

}

// Hash returns the header's hash, which identifies the whole block.
func (h Header) Hash() crypto.Hash {
	return crypto.HashFields(
		[]byte(h.ChainID),
		crypto.Uint64Bytes(h.Height),
		h.PrevHash[:],
		h.TxRoot[:],
		h.ValidatorHash[:],
		h.Proposer[:],
	)
}

// Block is a header plus the transaction it commits to.
type Block struct {
	Header Header
	Txs    [][]byte
}

// NewBlock builds a block and fills in TxRoot from txs.
func NewBlock(h Header, txs [][]byte) Block {
	h.TxRoot = crypto.MerkleRoot(txs)
	return Block{Header: h, Txs: txs}
}

// Hash returns the block's identity; the has of uts header.
func (b Block) Hash() crypto.Hash {
	return b.Header.Hash()
}

// ValidateBasic checks what can be checked without any other blocks.
func (b Block) ValidateBasic() error {
	if b.Header.ChainID == "" {
		return ErrEmptyChainID
	}
	if b.Header.Height == 0 {
		return ErrZeroHeight
	}
	if crypto.MerkleRoot(b.Txs) != b.Header.TxRoot {
		return ErrBadTxRoot
	}
	return nil
}
