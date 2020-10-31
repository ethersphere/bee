package postage

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// ErrOwnerMismatch is the error given for invalid signatures
	ErrOwnerMismatch = errors.New("owner mismatch")
	// ErrStampInvalid is the error given if stamp cannot deserialise
	ErrStampInvalid = errors.New("invalid stamp")
)

// Batch represents a postage batch, a payment on the blockchain
type Batch struct {
	ID     []byte   // batch ID
	Amount *big.Int // overall balance of the batch
	Start  *big.Int // blocknumber the batch was created
	Owner  []byte   // owner's ethereum address
	Depth  uint8    // batch depth, i.e., size = 2^{depth}
}

// MarshalBinary serialises a postage batch to a byte slice len 117
func (b *Batch) MarshalBinary() ([]byte, error) {
	out := make([]byte, 117)
	copy(out, b.ID)
	amount := b.Amount.Bytes()
	copy(out[64-len(amount):], amount)
	start := b.Start.Bytes()
	copy(out[96-len(start):], start)
	copy(out[96:], b.Owner)
	out[116] = b.Depth
	return out, nil
}

// UnmarshalBinary deserialises the batch
// unsafe on slice index (len(buf) = 117) as only internally used in db
func (b *Batch) UnmarshalBinary(buf []byte) error {
	b.ID = buf[:32]
	b.Amount = big.NewInt(0).SetBytes(buf[32:64])
	b.Start = big.NewInt(0).SetBytes(buf[64:96])
	b.Owner = buf[96:116]
	b.Depth = buf[116]
	return nil
}

// Valid checks the validity of the postage stamp; in particular:
// - authenticity - check batch is valid on the blockchain
// - authorisation - the batch owner is the stamp signer
// the validity  check is only meaningful in its association of a chunk
// this chunk address needs to be given as argument
func (s *Stamp) Valid(addr swarm.Address) error {
	toSign, err := toSignDigest(addr, s.ID)
	if err != nil {
		return err
	}
	pk, err := crypto.Recover(s.Sig, toSign)
	if err != nil {
		return err
	}
	owner, err := crypto.NewEthereumAddress(*pk)
	if err != nil {
		return err
	}
	if !bytes.Equal(s.Owner, owner) {
		return ErrOwnerMismatch
	}
	return nil
}

// Stamp represents a postage stamp as attached to a chunk
type Stamp struct {
	*Batch        // postage batch
	Sig    []byte // common r[32]s[32]v[1]-style 65 byte ECDSA signature
}

// MarshalBinary gives the byte slice serialisation of a stamp: batchID[32]|Signature[65]
func (s *Stamp) MarshalBinary() ([]byte, error) {
	return append(append([]byte{}, s.ID...), s.Sig...), nil
}

// UnmarshalBinary parses a serialised stamp into id and signature
func (s *Stamp) UnmarshalBinary(buf []byte) error {
	if len(buf) != 97 {
		return ErrStampInvalid
	}
	s.Batch = &Batch{ID: buf[:32]}
	s.Sig = buf[32:]
	return nil
}

// toSignDigest creates a digest to represent the stamp which is to be signed by the owner
func toSignDigest(addr swarm.Address, id []byte) ([]byte, error) {
	h := swarm.NewHasher()
	_, err := h.Write(addr.Bytes())
	if err != nil {
		return nil, err
	}
	_, err = h.Write(id)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
