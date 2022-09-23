package api

import (
	"encoding/hex"
	"errors"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/google/go-cmp/cmp"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type (
	parseInt8Test struct {
		Int8Val int8 `parse:"int8Val"`
	}
	parseInt16Test struct {
		Int16Val int16 `parse:"int16Val"`
	}
	parseInt32Test struct {
		Int32Val int32 `parse:"int32Val"`
	}
	parseInt64Test struct {
		Int64Val int64 `parse:"int64Val"`
	}
	parseUint8Test struct {
		Uint8Val uint8 `parse:"uint8Val"`
	}
	parseUint16Test struct {
		Uint16Val uint16 `parse:"uint16Val"`
	}
	parseUint32Test struct {
		Uint32Val uint32 `parse:"uint32Val"`
	}
	parseUint64Test struct {
		Uint64Val uint64 `parse:"uint64Val"`
	}
	parseBatchIdTest struct {
		Id []byte `parse:"batch_id,hexStringToBytes"`
	}
	parseAddressTest struct {
		Address []byte `parse:"address,addressToBytes"`
	}
)

func Test_parse(t *testing.T) {
	t.Parallel()
	address := "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa"
	decodeString, _ := hex.DecodeString(address)
	tests := []struct {
		name    string
		src     interface{}
		want    interface{}
		wantErr error
	}{
		{
			name:    "int8",
			src:     url.Values{"int8Val": {"2"}},
			want:    &parseInt8Test{Int8Val: 2},
			wantErr: nil,
		}, {
			name:    "int16",
			src:     url.Values{"int16Val": {"2"}},
			want:    &parseInt16Test{Int16Val: 2},
			wantErr: nil,
		}, {
			name:    "int32",
			src:     url.Values{"int32Val": {"2"}},
			want:    &parseInt32Test{Int32Val: 2},
			wantErr: nil,
		},
		{
			name:    "int64",
			src:     url.Values{"int64Val": {"2"}},
			want:    &parseInt64Test{Int64Val: 2},
			wantErr: nil,
		},
		{
			name: "uInt8",
			src: map[string]string{
				"uint8Val": "1",
			},
			want:    &parseUint8Test{Uint8Val: 1},
			wantErr: nil,
		},
		{
			name: "uInt16",
			src: map[string]string{
				"uint16Val": "1",
			},
			want:    &parseUint16Test{Uint16Val: 1},
			wantErr: nil,
		},
		{
			name: "uInt32",
			src: map[string]string{
				"uint32Val": "1",
			},
			want:    &parseUint32Test{Uint32Val: 1},
			wantErr: nil,
		},
		{
			name: "uInt64",
			src: map[string]string{
				"uint64Val": "9",
			},
			want:    &parseUint64Test{Uint64Val: 9},
			wantErr: nil,
		},
		{
			name: "uInt64 different value",
			src: map[string]string{
				"uint64Val": "9",
			},
			want:    &parseUint64Test{Uint64Val: 8},
			wantErr: errors.New("invalid uint64Val"),
		},
		{
			name: "uInt64 alphabet input",
			src: map[string]string{
				"uint64Val": "mimiim",
			},
			want:    &parseUint64Test{Uint64Val: 8},
			wantErr: errors.New("invalid uint64Val"),
		},
		{
			name: "batchId",
			src: map[string]string{
				"uint8Val": hex.EncodeToString(postagetesting.MustNewBatch().ID),
			},
			want:    &parseBatchIdTest{Id: postagetesting.MustNewBatch().ID},
			wantErr: errors.New("invalid batch_id"),
		},
		{
			name: "address",
			src: map[string]string{
				"address": "bff2c89e85e78c38bd89fca1acc996afb876c21bf5a8482ad798ce15f1c223fa",
			},
			want:    &parseAddressTest{Address: decodeString},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			indirect := reflect.Indirect(reflect.ValueOf(tt.want))
			newIndirect := reflect.New(indirect.Type())

			// Set the value of the new element to the content of the old.
			newIndirect.Elem().Set(reflect.ValueOf(indirect.Interface()))
			have := newIndirect.Interface()
			haveErr := parse(tt.src, tt.want)
			if (haveErr != nil && tt.wantErr != nil && !strings.Contains(haveErr.Error(), tt.wantErr.Error())) || (haveErr != nil && tt.wantErr == nil) {
				t.Errorf("parse() error = %v, wantErr %v", haveErr, tt.wantErr)
			}
			diff := cmp.Diff(tt.want, have)
			if tt.wantErr == nil && diff != "" {
				t.Errorf("parse(...): result mismatch (-want +have):\n%s", diff)
			}
		})
	}
}
