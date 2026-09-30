package ledger

import (
	"errors"
	"math"
)

//Address identifies an account. On a real chain it is derived from a public key.

type Address string

//Errors a caller can check with errors.Is.

var (
	ErrInvalidAmount     = errors.New("amount must be greater than zero")
	ErrAccountNotFound   = errors.New("account not found")
	ErrInsufficientFunds = errors.New("insufficient Funds")
	ErrSelfTransfer      = errors.New("cannot transfer to self")
	ErrOverflow          = errors.New("balance overflow")
)

//Ledger holds every account balance, in the  smallest unit like upay.

type Ledger struct {
	balances map[Address]uint64
}

//New returns an empty, ready-to-use ledger.

func New() *Ledger {
	return &Ledger{balances: make(map[Address]uint64)}
}

//Balance return the balance of addr (0 if the account does not exist.)

func (l *Ledger) Balance(addr Address) uint64 {
	return l.balances[addr]
}

// Deposit adds amount to addr, creating the account if needed.
func (l *Ledger) Deposit(addr Address, amount uint64) error {
	if amount == 0 {
		return ErrInvalidAmount
	}
	if l.balances[addr] > math.MaxUint64-amount {
		return ErrOverflow
	}
	l.balances[addr] += amount
	return nil
}

//Transfer moves amount from one account to another. It is atomic:
//On any error, no balanc changes.

func (l *Ledger) Transfer(from, to Address, amount uint64) error {
	if amount == 0 {
		return ErrInvalidAmount
	}
	if from == to {
		return ErrSelfTransfer
	}
	bal, ok := l.balances[from]
	if !ok {
		return ErrAccountNotFound
	}
	if bal < amount {
		return ErrInsufficientFunds
	}
	if l.balances[to] > math.MaxUint64-amount {
		return ErrOverflow
	}
	l.balances[from] = bal - amount
	l.balances[to] += amount
	return nil
}
