//Package consensus implements Tendermint-style BFT consensus:vote counting,
// proposer selection and the propose/prevote/precommit state machine.

package consensus

//Quoram returns the smallest voting power that is more than 2/3 of toal.
//It never roverflows: it computes total - ceil(total/3)+1 instead of 2*total/3.

func Quorum(total uint64) uint64 {
	third := total / 3
	if total%3 != 0 {
		third++ //ceil(total/3)
	}
	return total - third + 1 //BREAK-POINT: REMOVE + 1
	// BREAK-POINT: return 2*total/3 + 1 (NAIVE FORMULA)
}

// HasQuorrum reprots whether the power is more than 2/3 of total.
func HasQuorum(power, total uint64) bool {
	return power >= Quorum(total)
}

//MaxFaulty returns the most Byzantine power a set of this total can tolerate:
// the largest f with 3f < total.

func MaxFaulty(total uint64) uint64 {
	if total == 0 {
		return 0
	}
	return (total - 1) / 3 //BREAK-POINT: return total / 3
}
