package api

import (
	"encoding/hex"
	"errors"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/google/go-cmp/cmp"
	"net/url"
	"strings"
	"testing"
)

type (
	parseInt64Test struct {
		Int64Val int64 `parse:"int64Val"`
	}

	parseUint32Test struct {
		UintVal uint32 `parse:"uint32Val"`
	}
	parseUint8Test struct {
		UintVal uint8 `parse:"uint8Val"`
	}
	parseBatchIdTest struct {
		Id []byte `parse:"id,hexToString" name:"batchID"`
	}
	parseAddressTest struct {
		Address []byte `parse:"address,addressToString"`
	}
)

func Test_parse(t *testing.T) {
	t.Parallel()
	b := postagetesting.MustNewBatch()
	queryVal := url.Values{}
	queryVal.Set("int64Val", "2")
	tests := []struct {
		name    string
		input   interface{}
		output  interface{}
		wantErr error
	}{
		{
			name:    "int64",
			input:   queryVal,
			output:  &parseInt64Test{Int64Val: 2},
			wantErr: nil,
		},
		{
			name: "uInt32",
			input: map[string]string{
				"uint32Val": "1",
			},
			output:  &parseUint32Test{UintVal: 1},
			wantErr: nil,
		},
		{
			name: "uInt8",
			input: map[string]string{
				"uint8Val": "1",
			},
			output:  &parseUint8Test{UintVal: 1},
			wantErr: nil,
		},
		{
			name: "batchId",
			input: map[string]string{
				"uint8Val": hex.EncodeToString(b.ID),
			},
			output:  &parseBatchIdTest{Id: b.ID},
			wantErr: errors.New("invalid batchID"),
		},
		{
			name: "address",
			input: map[string]string{
				"address": "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa",
			},
			output:  &parseAddressTest{Address: []byte("bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa")},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			have := tt.output
			if err := parse(tt.input, tt.output); err != nil && !strings.Contains(err.Error(), tt.wantErr.Error()) {
				t.Errorf("parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.output, have); diff != "" {
				t.Errorf("parse(...): result mismatch (-want +have):\n%s", diff)
			}
		})
	}
}
