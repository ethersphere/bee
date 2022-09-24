package lottery

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"

	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	ErrSlashed = errors.New("slashed")
)

// Sources are derived by the lottery contract from a **random seed**
// - truth selection source = `H(S|0)`
// - winner selection source = `H(S|1)`
// - neighbourhood selection source  = `H(S|2)`
// - reserve commitment source  = `H(S|3)`
// sources of randomness
const (
	Truth         int = iota // for truth selection
	Winner                   // for winner selection
	Neighbourhood            // for neighbourhood sellection
	RCSalt                   // for the RC calculation
)

// Contract is the interface for the lottery contract
type Contract interface {
	// contract start blockheight, round and phase lengths in blocks
	Config() (startBlock, blocksPerRound, BlocksPerPhase uint64)
	// accessors for random sources available in claim phase
	RandomSeedFor(int) ([]byte, error)
	// function that tells the node whether to claim
	IsWinner(overlay, truthSel, winnerSel []byte) (slashed, winner bool, err error)
	// function that tells the node whether to commit (start sampler)
	IsPlaying(overlay []byte, sd uint8, source []byte) (bool, error)
	// transactional calls
	Commit(rc []byte) error
	Reveal(sd uint8, rc, key []byte) error
	Claim() error
}

// DepthMonitor interface a dependency of the lottery agent to determine RC scope
type DepthMonitor interface {
	StorageDepth() uint8
}

// Sampler is an interface
//     the agent for the RC calculation
type Sampler interface {
	Calculate(salt []byte, sd uint8) ([]byte, error)
}

// Agent manages the storer node's participation in the postage lottery, i.e.,
// interacts through smart contract with the process for the redistribution of postage revenue
type Agent struct {
	Contract
	DepthMonitor
	Sampler
	overlay                  []byte
	result                   chan []byte
	quit                     chan struct{}
	commitPhase, revealPhase chan struct{}
}

// New constructs a new agent
func New(contract Contract, depthMonitor DepthMonitor, sampler Sampler, overlay []byte, blockHeightC chan uint64, quit chan struct{}) *Agent {
	a := &Agent{
		Contract:     contract,
		DepthMonitor: depthMonitor,
		Sampler:      sampler,
		result:       make(chan []byte, 1),
		quit:         quit,
		commitPhase:  make(chan struct{}, 1),
		revealPhase:  make(chan struct{}, 1),
	}
	go a.listen(blockHeightC)
	return a
}

// Listener is the process triggering actions relaetd to phases
// - Started after bootup if the node is a staked storer node
// - checked by calling the staking agent's get-staked-amount function
// - listens to block height of Ethereum blockchain and
// sets the lottery height (`LH`) as `height - startBlock`, where `startBlock` is the starting block of the lottery contract.
// - monitors the lottery phase changes
// - triggers the commit process if `LH % LI = 0`
// - triggers the reveal process if `LH % LI = LPL`
// - starts claim procedure if `LH % LI = 2*LPL`
// - where
// 	- *LI* is the lottery interval, and
// 	- *LPL* is the lottery (commit) phase length
func (a *Agent) listen(blockHeightC chan uint64) {
	startBlock, blocksPerRound, blocksPerPhase := a.Config()
	for {
		select {
		case h := <-blockHeightC:
			switch (h - startBlock) % blocksPerRound {
			case 0:
				a.commitPhase <- struct{}{}
			case blocksPerPhase:
				a.revealPhase <- struct{}{}
			case 2 * blocksPerPhase:
				go a.claim()
			default:
			}
		case <-a.quit:
			return
		}
	}
}

// ### claim procedure
// - Aborts any running sampler process since too late to commit/reveal
// - Checks if the node overlay is winning by calling a read only function of the contract passing the truth and winner selection sources.
// - if yes, then sends the claim transaction
// - Checks if the node is participating in the next round, i.e., its neighbourhood is selected, i.e., if the new **neighourhood selection source** is within the node's storage depth. If not it returns, otherwise
// - it starts the RC sampler process passing it the **reserve commitment source** and the storage depth.
//
func (a *Agent) claim() (playing bool, err error) {
	var isSlashed, isWinner bool
	truthRnd, err := a.RandomSeedFor(Truth)
	if err != nil {
		return false, err
	}
	winnerRnd, err := a.RandomSeedFor(Winner)
	if err != nil {
		return false, err
	}
	isSlashed, isWinner, err = a.IsWinner(a.overlay, truthRnd, winnerRnd)
	if err != nil {
		return false, err
	}
	if isSlashed {
		return false, ErrSlashed
	}
	if isWinner {
		if err := a.Claim(); err != nil {
			return false, err
		}
	}
	return a.initRound()
}

func (a *Agent) initRound() (playing bool, err error) {
	sd := a.StorageDepth()
	neighbourhoodRnd, err := a.RandomSeedFor(Neighbourhood)
	if err != nil {
		return false, err
	}
	isPlaying, err := a.IsPlaying(a.overlay, sd, neighbourhoodRnd)
	if err != nil {
		return false, err
	}
	if !isPlaying {
		return false, nil
	}

	salt, err := a.RandomSeedFor(RCSalt)
	if err != nil {
		return false, err
	}
	go a.RunCommitter(sd)
	return true, a.RunSampler(salt, sd)
}

// Sampler is the process calculating the Reserve Commitment
// - started by the claim procedure if the node's neighbourhood is selected for the coming lottery draw
// - aborted by the next claim procedure if not finished
// - starts the sampler passing it the current **storage depth**
// - starts the committer
// - if finishes it starts a new commit process passing it
// - the **reserve commitment hash** (`RC`) and
// - the storage depth (`SD`)
// - it quits
func (a *Agent) RunSampler(salt []byte, sd uint8) error {
	rc, err := a.Sampler.Calculate(salt, sd)
	// if the RC calculation quits (because it is takes too long and the reveal phase starts)
	// then the result channel is left empty
	if err != nil {
		return err
	}
	a.result <- rc
	return nil
}

// - started by the sampler
// - triggered by the listener when the commit phase starts.
// - generates a **Random Source** (`RN`, 32 bytes) to obfuscate the commit hash with
// - calculates the commit hash as `H(RC|SD|Overlay|RN)`
// - submits the commit hash by sending a transaction to the blockchain
// - triggers the async reveal process passing it `RC`,`SD` and `RN`
func (a *Agent) RunCommitter(sd uint8) error {
	var rc []byte
	var ready bool
	for !ready || rc == nil {
		select {
		case <-a.commitPhase:
			ready = true
		case rc = <-a.result:
		case <-a.quit:
			return nil
		}
	}
	// generate obfuscation key to encrypt the commit and to be revealed in the reveal phase
	key := make([]byte, swarm.HashSize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}
	orc, err := WrapCommit(sd, rc, a.overlay, key)
	if err != nil {
		return err
	}
	go a.RunRevealer(sd, rc, key)
	return a.Commit(orc)
}

//- started by the claim process if the node's neighbourhood is selected
// - triggered by the listener if reveal phase starts
// - submits `RC`, `SD`, `Overlay` and `RN` in a transaction to the blockchain
func (a *Agent) RunRevealer(sd uint8, rc, key []byte) error {
	select {
	case <-a.revealPhase:
	case <-a.quit:
		return nil
	}
	return a.Reveal(sd, rc, key)
}

// WrapCommit concatenates the byte serialisations of all the data needed to apply to
// the lottery and obfuscates it with a nonce that is to be revealed in the subsequent phase
// This should be a contract accessor taking `SD`, `RC` and overlay and the obfuscater nonce
func WrapCommit(sd uint8, rc, overlay, key []byte) (orc []byte, err error) {
	h := swarm.NewHasher()
	if _, err = h.Write(rc); err != nil {
		return nil, err
	}
	sdb := make([]byte, 8)
	binary.BigEndian.PutUint64(sdb, uint64(sd))
	if _, err = h.Write(sdb); err != nil {
		return nil, err
	}
	if _, err = h.Write(overlay); err != nil {
		return nil, err
	}
	if _, err = h.Write(key); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
