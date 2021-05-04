package bigint

import (
	"encoding/json"
	"fmt"
	"math/big"
)

type BigInt struct {
	big.Int
}

func (i BigInt) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, i.String())), nil
}

func (i *BigInt) UnmarshalJSON(b []byte) error {
	var val string
	err := json.Unmarshal(b, &val)
	if err != nil {
		return err
	}

	i.SetString(val, 10)

	return nil
}

func NewBigInt(x int64) *BigInt {
	b := new(BigInt)
	b.SetInt64(x)
	return b
}

func Wrap(i *big.Int) *BigInt {
	return &BigInt{*i}
}
