package ledger

import (
	"errors"
	"math"
	"testing"
)

//Setup returns a ledger pre-load with the given balances.

func setup(t *testing.T, start map[Address]uint64) *Ledger {
	t.Helper()
	l := New()
	for addr, amt := range start {
		if err := l.Deposit(addr, amt); err != nil {
			t.Fatalf("setup:deposit %d to %s:%v", amt, addr, err)
		}
	}
	return l
}

func TestDeposit(t *testing.T) {
	tests := []struct {
		name    string
		start   map[Address]uint64
		amount  uint64
		wantErr error
		wantBal uint64
	}{
		{"new account", nil, 100, nil, 100},
		{"adds to existing", map[Address]uint64{"alice": 50}, 25, nil, 75},
		{"zero amount rejected", map[Address]uint64{"alice": 50}, 0, ErrInvalidAmount, 50},
		{"overflow rejected", map[Address]uint64{"alice": math.MaxUint64}, 1, ErrOverflow, math.MaxUint64},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := setup(t, tc.start)
			err := l.Deposit("alice", tc.amount)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v,want %v", err, tc.wantErr)
			}
			if got := l.Balance("alice"); got != tc.wantBal {
				t.Errorf("balance=%d,want%d", got, tc.wantBal)
			}
		})
	}

}

func TestTransfer(t *testing.T) {
	tests := []struct {
		name     string
		start    map[Address]uint64
		from, to Address
		amount   uint64
		wantErr  error
		want     map[Address]uint64 //balances after the call
	}{
		{
			name:  "success",
			start: map[Address]uint64{"alice": 100},
			from:  "alice", to: "bob", amount: 30,
			want: map[Address]uint64{"alice": 70, "bob": 30},
		},
		{
			name:  "send entire balance",
			start: map[Address]uint64{"alice": 100},
			from:  "alice", to: "bob", amount: 100,
			want: map[Address]uint64{"alice": 0, "bob": 100},
		},
		{
			name:  "zero amount balance",
			start: map[Address]uint64{"alice": 100},
			from:  "alice", to: "bob", amount: 0,
			wantErr: ErrInvalidAmount,
			want:    map[Address]uint64{"alice": 100, "bob": 0},
		},
		{
			name:  "self transfer",
			start: map[Address]uint64{"alice": 100},
			from:  "alice", to: "alice", amount: 10,
			wantErr: ErrSelfTransfer,
			want:    map[Address]uint64{"alice": 100},
		},
		{
			name:  "unknown sender",
			start: map[Address]uint64{"alice": 100},
			from:  "mallory", to: "alice", amount: 10,
			wantErr: ErrAccountNotFound,
			want:    map[Address]uint64{"alice": 100, "mallory": 0},
		},
		{
			name:  "insufficient funds",
			start: map[Address]uint64{"alice": 100},
			from:  "alice", to: "bob", amount: 101,
			wantErr: ErrInsufficientFunds,
			want:    map[Address]uint64{"alice": 100, "bob": 0},
		},
		{ //ATTACK: push bob's balance past the maximum so it wraps to a tiny number.
			name:  "receiver overflow attack",
			start: map[Address]uint64{"alice": 10, "bob": math.MaxUint64},
			from:  "alice", to: "bob", amount: 1,
			wantErr: ErrOverflow,
			want:    map[Address]uint64{"alice": 10, "bob": math.MaxUint64},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := setup(t, tc.start)
			err := l.Transfer(tc.from, tc.to, tc.amount)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err= %v, want %v", err, tc.wantErr)
			}
			//Atomicity : on error, "want" equals "start", so this proves nothing chnaged.
			for addr, wantBal := range tc.want {
				if got := l.Balance(addr); got != wantBal {
					t.Errorf("balance[%s]=%d,want %d", addr, got, wantBal)
				}
			}
		})
	}
}

// FuzzTransfer checks two invariants for ANY starting balances and amount:
//  1. atomicity:    a failed transfer changes nothing
//  2. conservation: a successful transfer moves exactly `amount`, so supply is unchanged

func FuzzTransfer(f *testing.F) {
	//Seed corpus : starting points the fuzzer mutates from.
	f.Add(uint64(100), uint64(0), uint64(30))            //normal
	f.Add(uint64(100), uint64(0), uint64(101))           //insufficient funds
	f.Add(uint64(10), uint64(math.MaxUint64), uint64(1)) //overflow attack
	f.Add(uint64(0), uint64(0), uint64(5))               //alice doesn't exist.

	f.Fuzz(func(t *testing.T, aliceStart, bobStart, amount uint64) {
		l := New()
		if aliceStart > 0 {
			if err := l.Deposit("alice", aliceStart); err != nil {
				t.Fatalf("Setup alice: %v", err)
			}
		}
		if bobStart > 0 {
			if err := l.Deposit("bob", bobStart); err != nil {
				t.Fatalf("Setup bob: %v", err)
			}
		}
		err := l.Transfer("alice", "bob", amount)
		aliceEnd, bobEnd := l.Balance("alice"), l.Balance("bob")

		if err != nil {
			//Invariant 1: atomicity
			if aliceEnd != aliceStart || bobEnd != bobStart {
				t.Fatalf("failed transfer chnaged state: alice %d->%d, bob %d->%d(err=%v)", aliceStart, aliceEnd, bobStart, bobEnd, err)
			}
			return
		}
		//Invariant 2 : conservation
		if aliceEnd > aliceStart || aliceStart-aliceEnd != amount {
			t.Fatalf("alice lost wrong amount: %d->%d, amount%d ", aliceStart, aliceEnd, amount)
		}
		if bobEnd < bobStart || bobEnd-bobStart != amount {
			t.Fatalf("bob gained wrong amount: %d->%d, amount %d", bobStart	, bobEnd, amount)
		}
	})
}
