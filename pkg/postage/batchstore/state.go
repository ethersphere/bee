package batchstore

import (
	"encoding"
	"encoding/binary"
	"math/big"

	"github.com/ethersphere/bee/pkg/storage"
)

var stateKey = "stateKey"

// state implements BinaryMarshaler interface
var _ encoding.BinaryMarshaler = (*state)(nil)

// state represents the current state of the reserve
type state struct {
	block uint64   // the block number of the last postage event
	total *big.Int // cumulative amount paid per stamp
	price *big.Int // bzz/chunk/block normalised price
}

// MarshalBinary serialises the state to be used by the state store
func (st *state) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 9)
	binary.BigEndian.PutUint64(buf, st.block)
	totalBytes := st.total.Bytes()
	buf[8] = uint8(len(totalBytes))
	buf = append(buf, totalBytes...)
	return append(buf, st.price.Bytes()...), nil
}

// UnmarshalBinary deserialises the state to be used by the state store
func (st *state) UnmarshalBinary(buf []byte) error {
	st.block = binary.BigEndian.Uint64(buf[:8])
	totalLen := int(buf[8])
	st.total = new(big.Int).SetBytes(buf[9 : 9+totalLen])
	st.price = new(big.Int).SetBytes(buf[9+totalLen:])
	return nil
}

// loads the state from statestore, initialises if not found
func (st *state) load(store storage.StateStorer) error {
	err := store.Get(stateKey, st)
	if err == storage.ErrNotFound {
		st.total = big.NewInt(0)
		st.price = big.NewInt(0)
		return nil
	}
	return err
}

func (st *state) save(store storage.StateStorer) error {
	return store.Put(stateKey, st)
}
