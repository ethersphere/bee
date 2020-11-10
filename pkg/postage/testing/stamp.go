package testing

import (
	crand "crypto/rand"
	"io"

	"github.com/ethersphere/bee/pkg/postage"
)

func NewStamp() *postage.Stamp {
	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		panic(err.Error())
	}
	sig := make([]byte, 65)
	_, err = io.ReadFull(crand.Reader, sig)
	if err != nil {
		panic(err.Error())
	}
	return postage.NewStamp(id, sig)
}
