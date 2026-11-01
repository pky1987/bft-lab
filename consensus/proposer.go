package consensus

import (
	"errors"
	"math"

	"github.com/pky1987/bft-lab/types"
)

// MaxTotalPower caps the set's total power so priority arithemetic doesn't overflow.
const MaxTotalPower = math.MaxInt64 / 8

// ErrPowerTooLarge is returned when a validator set exceeds MaxTotalPower.
var ErrPowerTooLarge = errors.New("total voting power exceeds the proposer limit")

//ProposerSelector picks a proposer by weighted round-robin.Every node that starts from the
//same validator set gets the same sequence, with no message exchnaged.

type ProposerSelector struct {
	vals     []types.Validator //sorted by address
	priority []int64           //priority[i] belongs to vals[i]
	total    int64             //sum of all validators' power
}

//NewProposerSelector returns a seleector with every priority at zero.

func NewProposerSelector(set *types.ValidatorSet) (*ProposerSelector, error) {
	if set.TotalPower() > MaxTotalPower {
		return nil, ErrPowerTooLarge
	}
	vals := set.Validators()
	return &ProposerSelector{
		vals:     vals,
		priority: make([]int64, len(vals)),
		total:    int64(set.TotalPower()),
	}, nil
}

// Next returns the proposer for next round:everyone gains their power, the heighest
// priority wins(lowest address a tie),and the winner pays the total power.
func (p *ProposerSelector) Next() types.Validator {
	best := 0
	for i, v := range p.vals {
		p.priority[i] += int64(v.Power)
		if p.priority[i] > p.priority[best] {
			best = i
		}
	}
	p.priority[best] -= p.total
	return p.vals[best]
}
