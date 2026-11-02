package consensus

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/pky1987/bft-lab/crypto"
	"github.com/pky1987/bft-lab/types"
)

func noTxs() [][]byte { return [][]byte{[]byte("tx")} }

// delivery is one message on its way to one node.
type delivery struct {
	to  int
	msg Message
}

// network connects nodes in memory. With a nil rng it is FIFO; otherwise order is shuffled.
type network struct {
	nodes  []*State
	queue  []delivery
	rng    *rand.Rand
	silent map[crypto.Address]bool // crashed nodes: never receive, never send
}

func newNetwork(t *testing.T, rng *rand.Rand, silent ...int) (*network, *types.ValidatorSet) {
	t.Helper()
	vals, keys := fourVals(t)
	n := &network{rng: rng, silent: map[crypto.Address]bool{}}
	for _, k := range keys {
		st, err := NewState(chain, k, vals, noTxs)
		if err != nil {
			t.Fatal(err)
		}
		n.nodes = append(n.nodes, st)
	}
	for _, i := range silent {
		n.silent[vals.Validators()[i].Address] = true
	}
	return n, vals
}

// broadcast queues every message for every node, including the sender.
func (n *network) broadcast(msgs []Message) {
	for _, m := range msgs {
		for i := range n.nodes {
			n.queue = append(n.queue, delivery{to: i, msg: m})
		}
	}
}

// run delivers messages until every live node passes maxHeight or nothing is left to deliver.
// A halted chain just empties the queue, so tests must check how far the nodes got.
func (n *network) run(t *testing.T, maxHeight uint64) {
	t.Helper()
	for _, st := range n.nodes {
		if !n.silent[st.addr] {
			n.broadcast(st.Start())
		}
	}
	for len(n.queue) > 0 && !n.done(maxHeight) {
		i := 0
		if n.rng != nil {
			i = n.rng.IntN(len(n.queue))
		}
		d := n.queue[i]
		n.queue = append(n.queue[:i], n.queue[i+1:]...)
		st := n.nodes[d.to]
		if n.silent[st.addr] {
			continue
		}
		out, err := st.Handle(d.msg)
		if err != nil {
			t.Fatalf("node %d: %v", d.to, err)
		}
		n.broadcast(out)
	}
}

// done reports whether every live node has passed maxHeight.
func (n *network) done(maxHeight uint64) bool {
	for _, st := range n.nodes {
		if !n.silent[st.addr] && st.Height() <= maxHeight {
			return false
		}
	}
	return true
}

// checkAgreement fails unless every pair of nodes has the same block at every shared height.
func checkAgreement(t *testing.T, n *network) {
	t.Helper()
	for _, a := range n.nodes {
		for _, b := range n.nodes {
			ba, bb := a.Blocks(), b.Blocks()
			for h := 0; h < len(ba) && h < len(bb); h++ {
				if ba[h].Hash() != bb[h].Hash() {
					t.Fatalf("FORK at height %d", h+1)
				}
			}
		}
	}
}

// TestFourNodesCommit: 4 honest nodes commit identical, correctly linked blocks.
func TestFourNodesCommit(t *testing.T) {
	n, _ := newNetwork(t, nil)
	n.run(t, 5)
	checkAgreement(t, n)

	// The test should fail when a node falls short of 5 blocks.
	for i, st := range n.nodes {
		if len(st.Blocks()) < 5 {
			t.Fatalf("node %d has only %d blocks", i, len(st.Blocks()))
		}
	}

	var prev crypto.Hash // starts as zero: block 1 has no parent
	for i, b := range n.nodes[0].Blocks() {
		// The test should fail when a block has the wrong height.
		if b.Header.Height != uint64(i+1) {
			t.Fatalf("block %d has height %d", i+1, b.Header.Height)
		}
		// The test should fail when a block does not link to its previous block.
		if b.Header.PrevHash != prev {
			t.Fatalf("block %d has incorrect PrevHash", i+1)
		}
		prev = b.Hash()
	}
}

// TestProposersTakeTurns: with equal power, proposers rotate strictly in address order.
func TestProposersTakeTurns(t *testing.T) {
	n, vals := newNetwork(t, nil)
	n.run(t, 8)
	vs := vals.Validators()
	for i, b := range n.nodes[0].Blocks() {
		expected := vs[i%len(vs)].Address
		// The test should fail when a block was proposed by a validator whose turn it wasn't.
		if b.Header.Proposer != expected {
			t.Fatalf("height %d proposed by wrong validator", i+1)
		}
	}
}

// TestAgreementUnderShuffledDelivery: any message order still gives agreement and progress.
// The seed is in the name and the message, so any failure can be replayed exactly.
func TestAgreementUnderShuffledDelivery(t *testing.T) {
	for seed := uint64(1); seed <= 50; seed++ {
		t.Run(fmt.Sprintf("seed:%d", seed), func(t *testing.T) {
			n, _ := newNetwork(t, rand.New(rand.NewPCG(seed, 0)))
			n.run(t, 5)
			checkAgreement(t, n)

			for i, node := range n.nodes {
				st := node.Blocks()
				if len(st) < 5 {
					t.Fatalf("seed %d: node %d has only %d blocks", seed, i, len(st))
				}
			}
		})
	}
}

// TestOneSilentNodeHaltsAtItsTurn: node 2 would propose height 3, so without timeouts the
// others commit 2 blocks and then wait forever. Lesson 15 fixes this.
func TestOneSilentNodeHaltsAtItsTurn(t *testing.T) {
	n, _ := newNetwork(t, nil, 2)
	n.run(t, 10)
	checkAgreement(t, n)

	for i, node := range n.nodes {
		if i == 2 {
			continue
		}
		blocks := node.Blocks()
		if len(blocks) != 2 {
			t.Fatalf("node %d: expected 2 blocks, got %d", i, len(blocks))
		}
	}
}

// TestTwoSilentNodesNeverCommit: 2 of 4 can't reach the quorum of 3, so nothing is committed.
// Honest-only tests can't catch a missing quorum rule; this one can.
func TestTwoSilentNodesNeverCommit(t *testing.T) {
	n, _ := newNetwork(t, nil, 2, 3)
	n.run(t, 10)

	for i, node := range n.nodes {
		blocks := node.Blocks()
		if len(blocks) != 0 {
			t.Fatalf("node %d: expected 0 blocks, got %d", i, len(blocks))
		}
	}
}

// fixture is one non-proposer node at height 1, plus the keys and honest proposal around it.
// vs[0] proposes height 1, and the node under test is vs[1].
func fixture(t *testing.T) (*State, *types.ValidatorSet, map[crypto.Address]crypto.PrivateKey, types.Proposal) {
	t.Helper()
	vals, keys := fourVals(t)
	byAddr := map[crypto.Address]crypto.PrivateKey{}
	for _, k := range keys {
		byAddr[k.Public().Address()] = k
	}
	vs := vals.Validators()
	proposerKey := byAddr[vs[0].Address]
	st, err := NewState(chain, byAddr[vs[1].Address], vals, noTxs)
	if err != nil {
		t.Fatal(err)
	}
	if out := st.Start(); len(out) != 0 {
		t.Fatalf("non-proposer sent %d messages on start", len(out))
	}
	block := types.NewBlock(types.Header{ChainID: chain, Height: 1, ValidatorHash: vals.Hash(), Proposer: vs[0].Address}, noTxs())
	p := types.Proposal{Height: 1, Block: block}
	p.Sign(chain, proposerKey)
	return st, vals, byAddr, p
}

// signedVote builds a validly signed round 0 vote from k.
func signedVote(k crypto.PrivateKey, typ types.VoteType, height uint64, h crypto.Hash) Message {
	v := types.Vote{Type: typ, Height: height, BlockHash: h, Validator: k.Public().Address()}
	v.Sign(chain, k)
	return Message{Vote: &v}
}

// TestRejectsBadProposals: each bad proposal gets its exact error and no vote.
func TestRejectsBadProposals(t *testing.T) {
	_, vals, byAddr, good := fixture(t)
	vs := vals.Validators()
	proposerKey := byAddr[vs[0].Address]
	sign := func(p types.Proposal, k crypto.PrivateKey) types.Proposal {
		p.Sign(chain, k)
		return p
	}

	// Most cases re-sign. Without a valid signature, the signature check would reject them
	// first and the check we're aiming at would never run.
	wrongProposer := good
	wrongProposer.Block.Header.Proposer = vs[1].Address

	badSig := good
	badSig.Signature[0] ^= 1

	tampered := good
	tampered.Block.Txs = [][]byte{[]byte("EVIL")}

	wrongPrev := good
	wrongPrev.Block.Header.PrevHash = crypto.Sum([]byte("x"))

	wrongVals := good
	wrongVals.Block.Header.ValidatorHash = crypto.Sum([]byte("x"))

	wrongChain := good
	wrongChain.Block.Header.ChainID = "other-chain"

	wrongRound := good
	wrongRound.Round = 1

	cases := []struct {
		name string
		p    types.Proposal
		want error
	}{
		{"wrong proposer", sign(wrongProposer, byAddr[vs[1].Address]), ErrWrongProposer},
		{"bad signature", badSig, ErrBadProposalSig},
		{"txs not matching TxRoot", sign(tampered, proposerKey), ErrBadBlock},
		{"wrong parent", sign(wrongPrev, proposerKey), ErrBadBlock},
		{"wrong validator set", sign(wrongVals, proposerKey), ErrBadBlock},
		{"wrong chain", sign(wrongChain, proposerKey), ErrBadBlock},
		{"wrong round", sign(wrongRound, proposerKey), ErrWrongRound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, _, _, _ := fixture(t) // a fresh node, so one case can't affect the next
			out, err := st.Handle(Message{Proposal: &c.p})
			if !errors.Is(err, c.want) {
				t.Fatalf("err=%v, want %v", err, c.want)
			}
			if len(out) != 0 {
				t.Fatal("node voted for a bad proposal")
			}
		})
	}
}

// TestProposalDuplicateAndConflict: the same block again is ignored, a different block is an error.
func TestProposalDuplicateAndConflict(t *testing.T) {
	st, vals, byAddr, p := fixture(t)

	// First time: the node prevotes.
	if out, err := st.Handle(Message{Proposal: &p}); err != nil || len(out) != 1 || out[0].Vote == nil {
		t.Fatalf("first proposal: out=%d err=%v, want 1 vote", len(out), err)
	}
	// Same block again: normal on a gossip network, so no error and no second vote.
	if out, err := st.Handle(Message{Proposal: &p}); err != nil || len(out) != 0 {
		t.Fatalf("duplicate proposal: out=%d err=%v, want nothing", len(out), err)
	}
	// A different block from the real proposer: equivocation.
	other := types.NewBlock(p.Block.Header, [][]byte{[]byte("other")})
	q := types.Proposal{Height: 1, Block: other}
	q.Sign(chain, byAddr[vals.Validators()[0].Address])
	if _, err := st.Handle(Message{Proposal: &q}); !errors.Is(err, ErrConflictingProposal) {
		t.Fatalf("err=%v, want ErrConflictingProposal", err)
	}
}

// TestPrecommitNeedsPrevoteQuorum: no precommit until 3 of 4 prevotes for the block.
func TestPrecommitNeedsPrevoteQuorum(t *testing.T) {
	st, vals, byAddr, p := fixture(t)
	vs := vals.Validators()
	h := p.Block.Hash()
	if _, err := st.Handle(Message{Proposal: &p}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		out, err := st.Handle(signedVote(byAddr[vs[i].Address], types.Prevote, 1, h))
		if err != nil || len(out) != 0 {
			t.Fatalf("prevote %d: out=%d err=%v, want no precommit yet", i, len(out), err)
		}
	}
	out, err := st.Handle(signedVote(byAddr[vs[2].Address], types.Prevote, 1, h))
	if err != nil || len(out) != 1 || out[0].Vote == nil || out[0].Vote.Type != types.Precommit {
		t.Fatalf("third prevote: out=%d err=%v, want 1 precommit", len(out), err)
	}
}

// TestCommitNeedsPrecommitQuorumAndTheBlock: votes alone can't commit a block the node hasn't seen.
func TestCommitNeedsPrecommitQuorumAndTheBlock(t *testing.T) {
	st, vals, byAddr, p := fixture(t)
	vs := vals.Validators()
	h := p.Block.Hash()

	// All precommits arrive before the proposal, so the node must wait.
	for i := 0; i < 3; i++ {
		if _, err := st.Handle(signedVote(byAddr[vs[i].Address], types.Precommit, 1, h)); err != nil {
			t.Fatal(err)
		}
		if st.Height() != 1 {
			t.Fatal("committed without having the block")
		}
	}
	// The block arrives: the quorum is already there, so it commits straight away.
	if _, err := st.Handle(Message{Proposal: &p}); err != nil {
		t.Fatal(err)
	}
	if st.Height() != 2 || len(st.Blocks()) != 1 || st.Blocks()[0].Hash() != h {
		t.Fatalf("height=%d blocks=%d, want height 2 with the block committed", st.Height(), len(st.Blocks()))
	}
}

// TestEarlyAndOldMessages: future messages wait and replay, old ones are ignored without error.
func TestEarlyAndOldMessages(t *testing.T) {
	st, vals, byAddr, p := fixture(t)
	vs := vals.Validators()
	h := p.Block.Hash()

	// Height 2 messages wait, without being checked yet.
	future := crypto.Sum([]byte("future"))
	early := signedVote(byAddr[vs[0].Address], types.Prevote, 2, future)
	if out, err := st.Handle(early); err != nil || len(out) != 0 {
		t.Fatalf("early vote: out=%d err=%v", len(out), err)
	}
	badEarly := signedVote(byAddr[vs[0].Address], types.Prevote, 2, future)
	badEarly.Vote.Signature[0] ^= 1
	if _, err := st.Handle(badEarly); err != nil {
		t.Fatal(err)
	}
	earlyProp := types.Proposal{Height: 2, Block: p.Block}
	if _, err := st.Handle(Message{Proposal: &earlyProp}); err != nil {
		t.Fatal(err)
	}
	if len(st.pending) != 3 {
		t.Fatalf("pending=%d, want 3", len(st.pending))
	}

	// Commit height 1, which replays them. Only the good vote counts, so prevotes is 1, not 2.
	if _, err := st.Handle(Message{Proposal: &p}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := st.Handle(signedVote(byAddr[vs[i].Address], types.Precommit, 1, h)); err != nil {
			t.Fatal(err)
		}
	}
	if st.Height() != 2 || st.prevotes.Size() != 1 || len(st.pending) != 0 {
		t.Fatalf("height=%d replayed prevotes=%d pending=%d, want 2, 1 and 0", st.Height(), st.prevotes.Size(), len(st.pending))
	}

	// Old messages are ignored without error.
	if out, err := st.Handle(signedVote(byAddr[vs[0].Address], types.Precommit, 1, h)); err != nil || len(out) != 0 {
		t.Fatalf("old vote: out=%d err=%v", len(out), err)
	}
	if out, err := st.Handle(Message{Proposal: &p}); err != nil || len(out) != 0 {
		t.Fatalf("old proposal: out=%d err=%v", len(out), err)
	}
}

// TestVoteErrorsAreReported: a bad vote for the current height comes back as VoteSet's error.
func TestVoteErrorsAreReported(t *testing.T) {
	st, vals, byAddr, p := fixture(t)
	m := signedVote(byAddr[vals.Validators()[0].Address], types.Prevote, 1, p.Block.Hash())
	m.Vote.Signature[0] ^= 1
	if _, err := st.Handle(m); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err=%v, want ErrBadSignature", err)
	}
}

// TestNewStateRejects: a key outside the set, and a set too large for proposer selection.
func TestNewStateRejects(t *testing.T) {
	vals, _ := fourVals(t)
	if _, err := NewState(chain, key(t, 9), vals, noTxs); !errors.Is(err, ErrNotValidator) {
		t.Fatalf("err=%v, want ErrNotValidator", err)
	}
	k := key(t, 1)
	huge, err := types.NewValidatorSet([]types.Validator{types.NewValidator(k.Public(), 1<<62)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewState(chain, k, huge, noTxs); !errors.Is(err, ErrPowerTooLarge) {
		t.Fatalf("err=%v, want ErrPowerTooLarge", err)
	}
}
