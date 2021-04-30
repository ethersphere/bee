package bigint

import (
	"fmt"
	"math/big"
)

type BigInt struct {
	big.Int
}

func (i BigInt) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, i.String())), nil
}

func NewBigInt(x int64) *BigInt {
	b := new(BigInt)
	b.SetInt64(x)
	return b
}

func Wrap(i *big.Int) *BigInt {
	return &BigInt{*i}
}
