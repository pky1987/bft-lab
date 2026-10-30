package consensus

import (
	"math"
	"testing"
)

func TestQuorumTable(t *testing.T) {
	cases := []struct {
		total uint64
		want  uint64
	}{
		{total: 1, want: 1},
		{total: 3, want: 3},
		{total: 4, want: 3},
		{total: 7, want: 5},
		{total: 10, want: 7},
		{total: 100, want: 67},
	}

	for _, c := range cases {
		if got := Quorum(c.total); got != c.want {
			t.Fatalf("Quorum(%d) = %d, want %d",
				c.total, got, c.want)
		}
	}
}

func TestQuorumIsSmallestMoreThanTwoThirds(t *testing.T) {
	for total := uint64(1); total <= 10000; total++ {
		q := Quorum(total)
		if 3*q <= 2*total {
			t.Fatalf("Quorum(%d)=%d ; not more than 2/3", total, q)
		}
		if 3*(q-1) > 2*total {
			t.Fatalf("Quorum(%d)=%d:not the smallest", total, q)
		}

	}
}

func TestQuorumsIntersectInAnHonestValidator(t *testing.T) {
	for total := uint64(1); total <= 10000; total++ {
		q := Quorum(total)
		f := MaxFaulty(total)

		if 2*q-total <= f {
			t.Fatalf("total=%d,quorum=%d,overlap=%d,maxFaulty=%d", total, q, 2*q-total, f)
		}
	}
}

func TestQuorumNoOverflow(t *testing.T) {
	maxPower := uint64(math.MaxUint64)
	q := Quorum(maxPower)
	//Claim A:everyone voting must satisfy quorum.
	if !HasQuorum(maxPower, maxPower) {
		t.Fatal("everyone voting must have quorum")
	}
	// Claim B: onevote below quorum must fail.
	if HasQuorum(q-1, maxPower) {
		t.Fatal("one below quorum must not have quorum")
	}

	//Claim C: quorum must not be tiny.
	if q < maxPower/3*2 {
		t.Fatalf("quorum too small:got %d", q)
	}
}

func TestMaxFaulty(t *testing.T) {
	cases := []struct {
		total uint64
		want  uint64
	}{
		{total: 0, want: 0},
		{total: 1, want: 0},
		{total: 3, want: 0},
		{total: 4, want: 1},
		{total: 7, want: 2},
		{total: 10, want: 3},
	}
	for _, c := range cases {
		if got := MaxFaulty(c.total); got != c.want {
			t.Fatalf("MaxFaulty(%d)=%d,want %d", c.total, got, c.want)
		}
	}
}
