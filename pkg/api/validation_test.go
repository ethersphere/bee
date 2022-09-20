package api

import (
	"net/http"
	"testing"
)

type (
	parseInt64Test struct {
		_ struct{}

		Int64Val int64 `parse:"int64Val"`
	}

	parseUint32Test struct {
		_ struct{}

		UintVal uint32 `parse:"uintVal"`
	}
)

func Test_parse(t *testing.T) {
	tests := []struct {
		name    string
		input   *http.Request
		output  interface{}
		wantErr error
	}{
		{
			name: "uInt",
			input: &http.Request{
				Form: map[string][]string{
					"uintVal": {"123"},
				},
			},
			output:  &parseUint32Test{},
			wantErr: nil,
		},
		{
			name: "int64",
			input: &http.Request{
				PostForm: map[string][]string{
					"int64Val": {"123"},
				},
			},
			output:  &parseInt64Test{},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := parse(tt.input, tt.output); err != tt.wantErr {
				t.Errorf("parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			//if diff := cmp.Diff(tt.want, have); diff != "" {
			//	t.Errorf("parse(...): result mismatch (-want +have):\n%s", diff)
			//}
		})
	}
}
