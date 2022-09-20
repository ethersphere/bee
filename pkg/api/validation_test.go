package api

import (
	"github.com/google/go-cmp/cmp"
	"testing"
)

type (
	parseInt64Test struct {
		Int64Val int64 `parse:"int64Val"`
	}

	parseUint32Test struct {
		UintVal uint32 `parse:"uintVal"`
	}
)

func Test_parse(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		output  interface{}
		wantErr error
	}{
		{
			name: "int64",
			input: map[string][]string{
				"int64Val": {"2"},
			},
			output:  &parseInt64Test{Int64Val: 2},
			wantErr: nil,
		},
		{
			name: "uInt32",
			input: map[string]string{
				"uintVal": "1",
			},
			output:  &parseUint32Test{UintVal: 1},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			have := tt.output
			if err := parse(tt.input, tt.output); err != tt.wantErr {
				t.Errorf("parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.output, have); diff != "" {
				t.Errorf("parse(...): result mismatch (-want +have):\n%s", diff)
			}
		})
	}
}
