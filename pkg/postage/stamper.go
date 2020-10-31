package postage

import (
	"errors"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// ErrBucketFull is the error when a collision bucket is full
	ErrBucketFull = errors.New("bucket full")
)

// Stamper connects a stampissuer with a signer
// a Stamper is created for each upload session
type Stamper struct {
	batch  *Batch
	issuer *StampIssuer
	signer crypto.Signer
}

// NewStamper constructs a Stamper
func NewStamper(b *Batch, st *StampIssuer, signer crypto.Signer) *Stamper {
	return &Stamper{b, st, signer}
}

// Stamp takes chunk, see if the chunk can included in the batch
// signs it with the owner of the batch of this Stamp issuer
func (st *Stamper) Stamp(addr swarm.Address) (*Stamp, error) {
	if err := st.issuer.inc(addr); err != nil {
		return nil, err
	}
	toSign, err := toSignDigest(addr, st.batch.ID)
	if err != nil {
		return nil, err
	}
	sig, err := st.signer.Sign(toSign)
	if err != nil {
		return nil, err
	}
	return &Stamp{st.batch, sig}, nil
}
