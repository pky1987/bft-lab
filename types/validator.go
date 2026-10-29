// Package types defines the data that validators agree on: validator sets, blocks and signed votes.
package types

import (
	"bytes"
	"errors"
	"github.com/pky1987/bft-lab/crypto"
	"math"
	"slices"
)

//Validator-set errors

var (
	ErrEmptyValidatorSet  = errors.New("validator is empty")
	ErrZeroPower          = errors.New("validator voting power must be positive")
	ErrDuplicateValidator = errors.New("duplicate validator")
	ErrPowerOverflow      = errors.New("total voting power overflows")
)

// Validator is one member of validator set.
type Validator struct {
	Address crypto.Address
	PubKey  crypto.PublicKey
	Power   uint64 // Voting Power: how much validator's vote counts
}

// NewValidator derives the address from the public key, so they can never disagree.
func NewValidator(pub crypto.PublicKey, power uint64) Validator {
	return Validator{Address: pub.Address(), PubKey: pub, Power: power}
}

// ValidatorSet is the validators for one height, sorted by address.
type ValidatorSet struct {
	validators []Validator
	total      uint64
}

//NewValidatorset validates vals and sorts them by address, so every node builds an identical
// set no matter what order the input came in.

func NewValidatorSet(vals []Validator) (*ValidatorSet, error) {
	if len(vals) == 0 {
		return nil, ErrEmptyValidatorSet
	}
	sorted := slices.Clone(vals)
	slices.SortFunc(sorted, func(a, b Validator) int {
		return bytes.Compare(a.Address[:], b.Address[:])
	})
	var total uint64
	for i, v := range sorted {
		if v.Power == 0 {
			return nil, ErrZeroPower
		}
		if i > 0 && v.Address == sorted[i-1].Address {
			return nil, ErrDuplicateValidator
		}
		if total > math.MaxUint64-v.Power {
			return nil, ErrPowerOverflow
		}
		total += v.Power
	}
	return &ValidatorSet{validators: sorted, total: total}, nil
}

// Size returns the number of validators.
func (s *ValidatorSet) Size() int {
	return len(s.validators)
}

// TotalPower returns  the sum of all power.
func (s *ValidatorSet) TotalPower() uint64 {
	return s.total
}

//Validators returns a copy of the validators, in address order.

func (s *ValidatorSet) Validators() []Validator {
	return slices.Clone(s.validators)
}

// GetByAddress finds a validator by address.
func (s *ValidatorSet) GetByAddress(addr crypto.Address) (Validator, bool) {
	for _, v := range s.validators {
		if v.Address == addr {
			return v, true
		}
	}
	return Validator{}, false
}

// Hash commits to every validator's key and power( a merkle root, in address order)
func (s *ValidatorSet) Hash() crypto.Hash {
	leaves := make([][]byte, len(s.validators))
	for i, v := range s.validators {
		h := crypto.HashFields(v.PubKey[:], crypto.Uint64Bytes(v.Power))
		leaves[i] = h[:]
	}
	return crypto.MerkleRoot(leaves)
}
