package consensus

import (
	"errors"
	"github.com/pky1987/bft-lab/crypto"
	"github.com/pky1987/bft-lab/types"
)

//VoteSet errors

var (
	ErrWrongHeight      = errors.New("vote is for a different height")
	ErrWrongRound       = errors.New("vote is for a different round")
	ErrWrongType        = errors.New("vote is for a different step")
	ErrUnknownValidator = errors.New("vote from a validator not in set")
	ErrBadSignature     = errors.New("vote signature is invalid")
	ErrConflictingVote  = errors.New("validator already voted for a different block")
)

//VoteSet collects the votes oof one(height,round,type) and track how much power backs
//each block hash. The zero hash counts as  a nil votes.

type VoteSet struct {
	chainID string
	height  uint64
	round   uint64
	typ     types.VoteType
	vals    *types.ValidatorSet

	votes   map[crypto.Address]types.Vote //atmost one vote per validator
	byBlock map[crypto.Hash]uint64        // power per block hash
	sum     uint64                        //power of everyone who voted
}

//NewVoteset returns an empty vote set for one height, roun and step.

func NewVoteSet(chainID string, height, round uint64, typ types.VoteType, vals *types.ValidatorSet) *VoteSet {
	return &VoteSet{
		chainID: chainID,
		height:  height,
		round:   round,
		typ:     typ,
		vals:    vals,
		votes:   make(map[crypto.Address]types.Vote),
		byBlock: make(map[crypto.Hash]uint64),
	}
}

//AddVote checks v and counts it. It returns false(and no error) for an exact duplicate.
// A second, different vote from the same validator is equivocation: ErrConflictingVote.

func (s *VoteSet) AddVote(v types.Vote) (bool, error) {
	if v.Height != s.height {
		return false, ErrWrongHeight
	}
	if v.Round != s.round {
		return false, ErrWrongRound
	}
	if v.Type != s.typ {
		return false, ErrWrongType
	}
	val, ok := s.vals.GetByAddress(v.Validator)
	if !ok {
		return false, ErrUnknownValidator
	}
	if !v.Verify(s.chainID, val.PubKey) {
		return false, ErrBadSignature
	}
	if old, seen := s.votes[v.Validator]; seen {
		if old.BlockHash == v.BlockHash {
			return false, nil //duplicate: gossip delives the same vote many times
		}
		return false, ErrConflictingVote
	}
	s.votes[v.Validator] = v
	s.byBlock[v.BlockHash] += val.Power
	s.sum += val.Power
	return true, nil
}

// TwoThirdsMajority returns the block hash that has more than 2/3 of the power, if any.
// The zero hash means 2/3+ voted nil.
func (s *VoteSet) TwoThirdsMajority() (crypto.Hash, bool) {
	for h, p := range s.byBlock {
		if HasQuorum(p, s.vals.TotalPower()) {
			return h, true
		}
	}
	return crypto.Hash{}, false
}

// HasTwoThirdsAny reports whether more than 2/3 of the power has voted,  for anything.
func (s *VoteSet) HasTwoThirdsAny() bool {
	return HasQuorum(s.sum, s.vals.TotalPower())
}

//Size returns how many validators has voted.

func (s *VoteSet) Size() int {
	return len(s.votes)
}
