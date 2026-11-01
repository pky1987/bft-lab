package consensus

import (
	"errors"
	"math"
	"testing"

	"github.com/pky1987/bft-lab/crypto"
	"github.com/pky1987/bft-lab/types"
)

func setOf(t *testing.T, powers ...uint64) *types.ValidatorSet {
	t.Helper()
	var vals []types.Validator
	for i, p := range powers {
		vals = append(vals, types.NewValidator(key(t, byte(i+1)).Public(), p))
	}
	s, err := types.NewValidatorSet(vals)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func selector(t *testing.T, s *types.ValidatorSet) *ProposerSelector {
	t.Helper()
	p, err := NewProposerSelector(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProposerEqualPowerRoundRobin(t *testing.T) {
	s := setOf(t, 1, 1, 1, 1)
	vs := s.Validators()
	p := selector(t, s)

	for i := 0; i < 20; i++ {
		expected := vs[i%4]
		actual := p.Next()
		if actual.Address != expected.Address {
			t.Fatalf("round %d :wrong proposer.", i)
		}
	}
}

func TestProposerFairShare(t *testing.T) {
	s := setOf(t, 1, 2, 3, 10)
	//Total Power=16 and 100 cycles = 1600 shares
	counts := make(map[crypto.Address]uint64)
	p := selector(t, s)
	for i := 0; i < 1600; i++ {
		v := p.Next()
		counts[v.Address]++
	}
	for _, v := range s.Validators() {
		expected := uint64(100) * v.Power
		if counts[v.Address] != expected {
			t.Fatalf("validator %X: expected %d shares, got %d", v.Address, expected, counts[v.Address])
		}
	}
}

func TestProposerDeterministic(t *testing.T) {
	a := setOf(t, 1, 2, 3, 10)
	b := setOf(t, 1, 2, 3, 10)
	p1 := selector(t, a)
	p2 := selector(t, b)

	for i := 0; i < 1000; i++ {
		proposerA := p1.Next()
		proposerB := p2.Next()

		if proposerA.Address != proposerB.Address {
			t.Fatalf("Selectors disagree at round %d", i)
		}
	}
}

func TestProposerPrioritySumToZero(t *testing.T) {
	a := setOf(t, 1, 3, 5, 9, 100)
	p := selector(t, a)
	for i := 0; i < 10000; i++ {
		p.Next()
		var sum int64
		for _, v := range p.priority {
			sum += v
			if v > p.total {
				t.Fatalf("priority %d exceeds positive total at round %d", v, i)
			}
			if v < -p.total {
				t.Fatalf("priority %d exceeds negative total at round %d", v, i)
			}
		}
		if sum != 0 {
			t.Fatalf("priority sum to %d at round %d", sum, i)
		}
	}
}

func TestProposerNoStarvation(t *testing.T) {
	s := setOf(t, 1, 1000)
	p := selector(t, s)
	var smallAddress crypto.Address
	for _, v := range s.Validators() {
		if v.Power == 1 {
			smallAddress = v.Address
			break
		}
	}
	if smallAddress == (crypto.Address{}) {
		t.Fatal("small validator not found")
	}
	found := false
	for i := 0; i < 1001; i++ {
		v := p.Next()
		if v.Address == smallAddress {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("small validator not found")
	}
}

func TestProposerRejectsHugePower(t *testing.T) {
	_, err := NewProposerSelector(setOf(t, math.MaxInt64))
	if !errors.Is(err, ErrPowerTooLarge) {
		t.Fatal("expected ErrPowerTooLarge, got", err)
	}
	if _, err := NewProposerSelector(setOf(t, MaxTotalPower)); err != nil {
		t.Fatalf("expected no error for MaxTotalPower, got %v", err)
	}

}
