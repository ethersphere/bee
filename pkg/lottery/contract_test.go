package lottery_test

type TestContract struct {
	config func() (startBlock, blocksPerRound, BlocksPerPhase uint64)
	// accessors for random sources available in claim phase
	randomSeedFor func(int) ([]byte, error)
	// function that tells the node whether to claim
	isWinner func(overlay, truthSel, winnerSel []byte) (slashed, winner bool, err error)
	// function that tells the node whether to commit (start sampler)
	isPlaying func(overlay []byte, sd uint8, source []byte) (bool, error)
	// transactional calls
	commit func(rc []byte) error
	reveal func(sd uint8, rc, key []byte) error
	claim  func() error
}

func (c *TestContract) WithConfig(f func() (startBlock, blocksPerRound, BlocksPerPhase uint64)) *TestContract {
	c.config = f
	return c
}

// accessors for random sources available in claim phase
func (c *TestContract) WithRandomSeedFor(f func(int) ([]byte, error)) *TestContract {
	c.randomSeedFor = f
	return c
}

// function that tells the node whether to claim
func (c *TestContract) WithIsWinner(f func(overlay, truthSel, winnerSel []byte) (slashed, winner bool, err error)) *TestContract {
	c.isWinner = f
	return c
}

// function that tells the node whether to commit (start sampler)
func (c *TestContract) WithIsPlaying(f func(overlay []byte, sd uint8, source []byte) (bool, error)) *TestContract {
	c.isPlaying = f
	return c
}

// transactional calls
func (c *TestContract) WithCommit(f func(rc []byte) error) *TestContract {
	c.commit = f
	return c
}

func (c *TestContract) WithReveal(f func(sd uint8, rc, key []byte) error) *TestContract {
	c.reveal = f
	return c
}

func (c *TestContract) WithClaim(f func() error) *TestContract {
	c.claim = f
	return c
}

// implementation using mockfunctions

func (c *TestContract) Config() (startBlock, blocksPerRound, BlocksPerPhase uint64) {
	return c.config()
}

func (c *TestContract) RandomSeedFor(i int) ([]byte, error) {
	return c.randomSeedFor(i)
}

func (c *TestContract) IsWinner(overlay, truthSel, winnerSel []byte) (slashed, winner bool, err error) {
	return c.isWinner(overlay, truthSel, winnerSel)
}

func (c *TestContract) IsPlaying(overlay []byte, sd uint8, neighbourSel []byte) (bool, error) {
	return c.isPlaying(overlay, sd, neighbourSel)
}

func (c *TestContract) Commit(rc []byte) error {
	return c.commit(rc)
}

func (c *TestContract) Reveal(sd uint8, rc, key []byte) error {
	return c.reveal(sd, rc, key)
}

func (c *TestContract) Claim() error {
	return c.claim()
}
