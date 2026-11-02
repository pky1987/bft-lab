package consensus

import (
	"errors"
	"slices"

	"github.com/pky1987/bft-lab/crypto"
	"github.com/pky1987/bft-lab/types"
)

// State machine errors
var (
	ErrNotValidator        = errors.New("key is not in the validator set")
	ErrWrongProposer       = errors.New("proposal is not from this round's proposer")
	ErrBadProposalSig      = errors.New("proposal signature is bad")
	ErrBadBlock            = errors.New("proposed block is invalid")
	ErrConflictingProposal = errors.New("proposer sent a different block")
)

// step is where a node is within one round.
type Step int

// The steps of a round. A node only moves forwards through them.
const (
	StepPropose Step = iota
	StepPrevote
	StepPrecommit
)

//Message is what a node broadcasts. Exactly one field is set.

type Message struct {
	Proposal *types.Proposal
	Vote     *types.Vote
}

//State is one validator's consensus state machine. It does no I/O and has no clock or
//goroutines: it takes a message in and returns the message to broadcast, including to itself.

type State struct {
	chainID  string
	key      crypto.PrivateKey
	addr     crypto.Address
	vals     *types.ValidatorSet
	selector *ProposerSelector
	txs      func() [][]byte //supplies the transaction when this node proposes

	height     uint64
	round      uint64
	step       Step
	proposer   types.Validator
	proposal   *types.Proposal
	prevotes   *VoteSet
	precommits *VoteSet

	lastHash crypto.Hash
	blocks   []types.Block
	pending  []Message //messages for future heights
}

// NewState returns a state machine waiting at height 1. Call Start to begin.
func NewState(chainID string, key crypto.PrivateKey, vals *types.ValidatorSet, txs func() [][]byte) (*State, error) {
	addr := key.Public().Address()
	if _, ok := vals.GetByAddress(addr); !ok {
		return nil, ErrNotValidator
	}
	selector, err := NewProposerSelector(vals)
	if err != nil {
		return nil, err
	}
	return &State{
		chainID:  chainID,
		key:      key,
		addr:     addr,
		vals:     vals,
		selector: selector,
		txs:      txs,
		height:   1,
	}, nil
}

// Start enters the first heightt and returns any message to broadcast.
func (s *State) Start() []Message {
	return s.enterHeight()
}

// Height returns the height thi node is currently deciding.
func (s *State) Height() uint64 {
	return s.height
}

//Block returns the committed chain.

func (s *State) Blocks() []types.Block {
	return slices.Clone(s.blocks)
}

// Handle processes one message and returns the message to boradcast, including to itself. It returns an error if the message is invalid or unexpected.
func (s *State) Handle(m Message) ([]Message, error) {
	if m.Proposal != nil {
		return s.handleProposal(m, *m.Proposal)
	}
	return s.handleVote(m, *m.Vote)
}

func (s *State) handleProposal(m Message, p types.Proposal) ([]Message, error) {
	if p.Height < s.height {
		return nil, nil //already committed that message
	}
	if p.Height > s.height {
		s.pending = append(s.pending, m)
		return nil, nil
	}
	if p.Round != s.round {
		return nil, ErrWrongRound
	}
	if p.Block.Header.Proposer != s.proposer.Address {
		return nil, ErrWrongProposer
	}
	if !p.Verify(s.chainID, s.proposer.PubKey) {
		return nil, ErrBadProposalSig
	}
	if s.proposal != nil {
		if s.proposal.Block.Hash() == p.Block.Hash() {
			return nil, nil //duplicate proposal with a different block: ignore it
		}
		return nil, ErrConflictingProposal //duplicate proposal with a different block: ignore it
	}
	if err := s.checkBlock(p.Block); err != nil {
		return nil, err
	}
	s.proposal = &p
	return s.advance(), nil

}

// checkBlock verifies that b is the right next block for this node's chain.
func (s *State) checkBlock(b types.Block) error {
	h := b.Header
	if b.ValidateBasic() != nil || h.ChainID != s.chainID || h.Height != s.height || h.PrevHash != s.lastHash || h.ValidatorHash != s.vals.Hash() {
		return ErrBadBlock
	}
	return nil
}

func (s *State) handleVote(m Message, v types.Vote) ([]Message, error) {
	if v.Height < s.height {
		return nil, nil
	}
	if v.Height > s.height {
		s.pending = append(s.pending, m)
		return nil, nil
	}
	set := s.prevotes
	if v.Type == types.Precommit {
		set = s.precommits
	}
	if _, err := set.AddVote(v); err != nil {
		return nil, err
	}
	return s.advance(), nil
}

// advnace moves the node forward as far as what it has been allows.
func (s *State) advance() []Message {
	if s.proposal == nil {
		return nil //can't advance without a proposal
	}
	var out []Message
	hash := s.proposal.Block.Hash()
	if s.step == StepPropose {
		s.step = StepPrevote
		out = append(out, s.vote(types.Prevote, hash))
	}
	if h, ok := s.prevotes.TwoThirdsMajority(); s.step == StepPrevote && ok && h == hash {
		s.step = StepPrecommit
		out = append(out, s.vote(types.Precommit, hash))
	}
	if h, ok := s.precommits.TwoThirdsMajority(); ok && h == hash {
		s.blocks = append(s.blocks, s.proposal.Block)
		s.lastHash = hash
		s.height++
		out = append(out, s.enterHeight()...)
	}
	return out
}

// enterHeight resets for s.height, proposes if it's this node's turn, then replays message
// that arrived early.
func (s *State) enterHeight() []Message {
	s.round = 0
	s.step = StepPropose
	s.proposal = nil
	s.prevotes = NewVoteSet(s.chainID, s.height, s.round, types.Prevote, s.vals)
	s.precommits = NewVoteSet(s.chainID, s.height, s.round, types.Precommit, s.vals)
	s.proposer = s.selector.Next()

	var out []Message
	if s.proposer.Address == s.addr {
		out = append(out, s.propose())
	}
	early := s.pending
	s.pending = nil
	for _, m := range early {
		// A bad early message is dropped here, exactly as it would be if it arrived now.
		if msgs, err := s.Handle(m); err == nil {
			out = append(out, msgs...)
		}
	}
	return out

}

func (s *State) propose() Message {
	block := types.NewBlock(types.Header{
		ChainID:       s.chainID,
		Height:        s.height,
		PrevHash:      s.lastHash,
		ValidatorHash: s.vals.Hash(),
		Proposer:      s.addr,
	},
		s.txs())
	p := types.Proposal{
		Height: s.height,
		Round:  s.round,
		Block:  block,
	}
	p.Sign(s.chainID, s.key)
	return Message{Proposal: &p}
}

func (s *State) vote(typ types.VoteType, block crypto.Hash) Message {
	v := types.Vote{
		Type:      typ,
		Height:    s.height,
		Round:     s.round,
		BlockHash: block,
		Validator: s.addr,
	}
	v.Sign(s.chainID, s.key)
	return Message{Vote: &v}
}
