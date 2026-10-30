package consensus

import (
	"bytes"
	"errors"
	"github.com/pky1987/bft-lab/crypto"
	"github.com/pky1987/bft-lab/types"
	"testing"
)

const chain = "bft-lab-1"

func key(t *testing.T, b byte) crypto.PrivateKey {
	t.Helper()
	k, err := crypto.NewKeyFromSeed(bytes.Repeat([]byte{b}, crypto.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	return k

}

//fourVals returns 4 equal-power validators (seed 1..4) and their keys.

func fourVals(t *testing.T) (*types.ValidatorSet, []crypto.PrivateKey) {
	t.Helper()
	var keys []crypto.PrivateKey
	var vals []types.Validator
	for b := byte(1); b <= 4; b++ {
		k := key(t, b)
		keys = append(keys, k)
		vals = append(vals, types.NewValidator(k.Public(), 1))
	}
	s, err := types.NewValidatorSet(vals)
	if err != nil {
		t.Fatal(err)
	}
	return s, keys
}

// vote returns a signed prevote at height 1, round 0
func vote(k crypto.PrivateKey, block crypto.Hash) types.Vote {
	v := types.Vote{Type: types.Prevote, Height: 1, Round: 0, BlockHash: block, Validator: k.Public().Address()}
	v.Sign(chain, k)
	return v
}

func TestVoteSetReachesQuorumAtThree(t *testing.T) {
	vals, keys := fourVals(t)
	s := NewVoteSet(chain, 1, 0, types.Prevote, vals)

	hashB := crypto.Sum([]byte("B"))
	for i := 0; i < 2; i++ {
		v := vote(keys[i], hashB)
		added, err := s.AddVote(v)
		if err != nil || !added {
			t.Fatalf("Add vote %d: added=%v err=%v", i, added, err)
		}
		_, ok := s.TwoThirdsMajority()
		if ok {
			t.Fatalf("majority reached after only %d votes", i+1)
		}
	}
	v := vote(keys[2], hashB)
	added, err := s.AddVote(v)
	if err != nil || !added {
		t.Fatalf("add third vote:added=%v err=%v", added, err)
	}
	got, ok := s.TwoThirdsMajority()
	if !ok {
		t.Fatal("expected two-third majority")
	}
	if got != hashB {
		t.Fatalf("majority hash =%x, want=%x", got, hashB)
	}
}

func TestVoteSetSplitHasNoMajority(t *testing.T) {
	vals, keys := fourVals(t)
	s := NewVoteSet(chain, 1, 0, types.Prevote, vals)

	hashA := crypto.Sum([]byte("A"))
	hashB := crypto.Sum([]byte("B"))

	// Two votes for A.
	for i := 0; i < 2; i++ {
		v := vote(keys[i], hashA)

		added, err := s.AddVote(v)
		if err != nil {
			t.Fatalf("Add vote for A: %v", err)
		}
		if !added {
			t.Fatal("expected vote for A to be added")
		}
	}

	// One vote for B.
	v := vote(keys[2], hashB)

	added, err := s.AddVote(v)
	if err != nil {
		t.Fatalf("Add vote for B: %v", err)
	}
	if !added {
		t.Fatal("expected vote for B to be added")
	}

	// Neither A nor B has a two-thirds majority.
	_, ok := s.TwoThirdsMajority()
	if ok {
		t.Fatal("unexpected two-thirds majority")
	}

	// Three out of four validators have voted.
	if !s.HasTwoThirdsAny() {
		t.Fatal("expected two-thirds of validators to have voted")
	}
}

func TestVoteSetNilMajority(t *testing.T) {
	vals, keys := fourVals(t)
	s := NewVoteSet(chain, 1, 0, types.Prevote, vals)
	nilHash := crypto.Hash{}
	for i := 0; i < 3; i++ {
		v := vote(keys[i], nilHash)
		added, err := s.AddVote(v)
		if err != nil {
			t.Fatalf("Add vote %d: added=%v err=%v", i, added, err)
		}
	}
	got, ok := s.TwoThirdsMajority()
	if !ok {
		t.Fatal("expected nil majority")
	}
	if !got.IsZero() {
		t.Fatalf("expected zero hash, got %x", got)
	}
}

func TestVoteSetDuplicateCountsOnce(t *testing.T) {
	vals, keys := fourVals(t)
	s := NewVoteSet(chain, 1, 0, types.Prevote, vals)

	hashA := crypto.Sum([]byte("A"))
	v := vote(keys[0], hashA)

	for i := 0; i < 5; i++ {
		added, err := s.AddVote(v)
		if err != nil {
			t.Fatalf("Add: %v", err)
		}

		if added != (i == 0) {
			t.Fatalf("pass %d: added=%v", i, added)
		}
	}

	if s.Size() != 1 {
		t.Fatalf("Size() = %d, want 1", s.Size())
	}

	if s.HasTwoThirdsAny() {
		t.Fatal("one vote must not be a two-thirds quorum")
	}
}

func TestVoteSetRejects(t *testing.T) {
	vals, keys := fourVals(t)

	hashA := crypto.Sum([]byte("A"))

	// Create valid votes first.
	wrongHeight := vote(keys[0], hashA)
	wrongHeight.Height = 2
	wrongHeight.Sign(chain, keys[0])

	wrongRound := vote(keys[0], hashA)
	wrongRound.Round = 1
	wrongRound.Sign(chain, keys[0])

	wrongType := vote(keys[0], hashA)
	wrongType.Type = types.Precommit
	wrongType.Sign(chain, keys[0])

	outsider := vote(key(t, 9), hashA)

	badSig := vote(keys[0], hashA)
	badSig.Signature[0] ^= 1

	cases := []struct {
		name string
		v    types.Vote
		want error
	}{
		{
			name: "wrong height",
			v:    wrongHeight,
			want: ErrWrongHeight,
		},
		{
			name: "wrong round",
			v:    wrongRound,
			want: ErrWrongRound,
		},
		{
			name: "wrong type",
			v:    wrongType,
			want: ErrWrongType,
		},
		{
			name: "outsider",
			v:    outsider,
			want: ErrUnknownValidator,
		},
		{
			name: "bad signature",
			v:    badSig,
			want: ErrBadSignature,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewVoteSet(chain, 1, 0, types.Prevote, vals)

			added, err := s.AddVote(tc.v)

			if added {
				t.Fatal("invalid vote was added")
			}

			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}

			if s.Size() != 0 {
				t.Fatalf("Size() = %d, want 0", s.Size())
			}
		})
	}
}

func TestVoteSetEquivocation(t *testing.T) {
	vals, keys := fourVals(t)
	s := NewVoteSet(chain, 1, 0, types.Prevote, vals)

	hashA := crypto.Sum([]byte("A"))
	hashB := crypto.Sum([]byte("B"))

	// First vote: keys[0] votes A.
	voteA := vote(keys[0], hashA)

	added, err := s.AddVote(voteA)
	if err != nil || !added {
		t.Fatalf("first vote: added=%v err=%v", added, err)
	}

	// Second vote: keys[0] tries B.
	voteB := vote(keys[0], hashB)

	added, err = s.AddVote(voteB)
	if added {
		t.Fatal("equivocating vote was added")
	}

	if !errors.Is(err, ErrConflictingVote) {
		t.Fatalf("error = %v, want ErrConflictingVote", err)
	}

	// Two other validators vote B.
	for _, i := range []int{1, 2} {
		v := vote(keys[i], hashB)

		added, err := s.AddVote(v)
		if err != nil || !added {
			t.Fatalf("validator %d: added=%v err=%v", i, added, err)
		}
	}

	// B has only two genuine votes.
	_, ok := s.TwoThirdsMajority()
	if ok {
		t.Fatal("unexpected majority for B")
	}

	if !s.HasTwoThirdsAny() {
		t.Fatal("expected three validators to have voted")
	}
}
