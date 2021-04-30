package bigint_test

import (
	"encoding/json"
	"github.com/ethersphere/bee/pkg/bigint"
	"reflect"
	"testing"
)

type TestStruct struct {
	Bg *bigint.BigInt `json:"bg"`
}

func TestMarshaling(t *testing.T) {
	mar, err := json.Marshal(TestStruct{Bg: bigint.NewBigInt(5)})

	if err != nil {
		t.Errorf("Marshaling failed")
	}

	if !reflect.DeepEqual(mar, []byte("{\"bg\":\"5\"}")) {
		t.Errorf("Wrongly marshaled data")
	}
}
