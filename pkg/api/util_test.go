// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type (
	parseBoolTest struct {
		BoolVal bool `parse:"boolVal"`
	}

	parseUintTest struct {
		UintVal uint `parse:"uintVal"`
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

	parseIntTest struct {
		IntVal int `parse:"intVal"`
	}

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

	parseFloat32Test struct {
		Float32Val float32 `parse:"float32Val"`
	}

	parseFloat64Test struct {
		Float64Val float64 `parse:"float64Val"`
	}

	parseByteSliceTest struct {
		ByteSliceVal []byte `parse:"byteSliceVal"`
	}

	parseStringTest struct {
		StringVal string `parse:"stringVal"`
	}
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     interface{}
		want    interface{}
		wantErr error
	}{{
		name: "bool zero value",
		src:  map[string]string{"boolVal": "0"},
		want: &parseBoolTest{},
	}, {
		name: "bool false",
		src:  map[string]string{"boolVal": "false"},
		want: &parseBoolTest{BoolVal: false},
	}, {
		name: "bool true",
		src:  map[string]string{"boolVal": "true"},
		want: &parseBoolTest{BoolVal: true},
	}, {
		name:    "bool syntax error",
		src:     map[string]string{"boolVal": "a"},
		want:    &parseBoolTest{},
		wantErr: api.NewParseError("boolVal", "a", strconv.ErrSyntax),
	}, {
		name: "uint zero value",
		src:  map[string]string{"uintVal": "0"},
		want: &parseUintTest{},
	}, {
		name: "uint in range value",
		src:  map[string]string{"uintVal": "1"},
		want: &parseUintTest{UintVal: 1},
	}, {
		name: "uint max value",
		src:  map[string]string{"uintVal": fmt.Sprintf("%d", uint(math.MaxUint))},
		want: &parseUintTest{UintVal: math.MaxUint},
	}, {
		name:    "uint out of range value",
		src:     map[string]string{"uintVal": "18446744073709551616"},
		want:    &parseUintTest{},
		wantErr: api.NewParseError("uintVal", "18446744073709551616", strconv.ErrRange),
	}, {
		name:    "uint syntax error",
		src:     map[string]string{"uintVal": "one"},
		want:    &parseUintTest{},
		wantErr: api.NewParseError("uintVal", "one", strconv.ErrSyntax),
	}, {
		name: "uint8 zero value",
		src:  map[string]string{"uint8Val": "0"},
		want: &parseUint8Test{},
	}, {
		name: "uint8 in range value",
		src:  map[string]string{"uint8Val": "10"},
		want: &parseUint8Test{Uint8Val: 10},
	}, {
		name: "uint8 max value",
		src:  map[string]string{"uint8Val": "255"},
		want: &parseUint8Test{Uint8Val: math.MaxUint8},
	}, {
		name:    "uint8 out of range value",
		src:     map[string]string{"uint8Val": "256"},
		want:    &parseUint8Test{},
		wantErr: api.NewParseError("uint8Val", "256", strconv.ErrRange),
	}, {
		name:    "uint8 syntax error",
		src:     map[string]string{"uint8Val": "ten"},
		want:    &parseUint8Test{},
		wantErr: api.NewParseError("uint8Val", "ten", strconv.ErrSyntax),
	}, {
		name: "uint16 zero value",
		src:  map[string]string{"uint16Val": "0"},
		want: &parseUint16Test{},
	}, {
		name: "uint16 in range value",
		src:  map[string]string{"uint16Val": "100"},
		want: &parseUint16Test{Uint16Val: 100},
	}, {
		name: "uint16 max value",
		src:  map[string]string{"uint16Val": "65535"},
		want: &parseUint16Test{Uint16Val: math.MaxUint16},
	}, {
		name:    "uint16 out of range value",
		src:     map[string]string{"uint16Val": "65536"},
		want:    &parseUint16Test{},
		wantErr: api.NewParseError("uint16Val", "65536", strconv.ErrRange),
	}, {
		name:    "uint16 syntax error",
		src:     map[string]string{"uint16Val": "hundred"},
		want:    &parseUint16Test{},
		wantErr: api.NewParseError("uint16Val", "hundred", strconv.ErrSyntax),
	}, {
		name: "uint32 zero value",
		src:  map[string]string{"uint32Val": "0"},
		want: &parseUint32Test{},
	}, {
		name: "uint32 in range value",
		src:  map[string]string{"uint32Val": "1000"},
		want: &parseUint32Test{Uint32Val: 1000},
	}, {
		name: "uint32 max value",
		src:  map[string]string{"uint32Val": "4294967295"},
		want: &parseUint32Test{Uint32Val: math.MaxUint32},
	}, {
		name:    "uint32 out of range value",
		src:     map[string]string{"uint32Val": "4294967296"},
		want:    &parseUint32Test{},
		wantErr: api.NewParseError("uint32Val", "4294967296", strconv.ErrRange),
	}, {
		name:    "uint32 syntax error",
		src:     map[string]string{"uint32Val": "thousand"},
		want:    &parseUint32Test{},
		wantErr: api.NewParseError("uint32Val", "thousand", strconv.ErrSyntax),
	}, {
		name: "uint64 zero value",
		src:  map[string]string{"uint64Val": "0"},
		want: &parseUint64Test{},
	}, {
		name: "uint64 in range value",
		src:  map[string]string{"uint64Val": "10000"},
		want: &parseUint64Test{Uint64Val: 10000},
	}, {
		name: "uint64 max value",
		src:  map[string]string{"uint64Val": "18446744073709551615"},
		want: &parseUint64Test{Uint64Val: math.MaxUint64},
	}, {
		name:    "uint64 out of range value",
		src:     map[string]string{"uint64Val": "18446744073709551616"},
		want:    &parseUint64Test{},
		wantErr: api.NewParseError("uint64Val", "18446744073709551616", strconv.ErrRange),
	}, {
		name:    "uint64 syntax error",
		src:     map[string]string{"uint64Val": "ten thousand"},
		want:    &parseUint64Test{},
		wantErr: api.NewParseError("uint64Val", "ten thousand", strconv.ErrSyntax),
	}, {
		name: "int zero value",
		src:  map[string]string{"intVal": "0"},
		want: &parseIntTest{},
	}, {
		name: "int in range value",
		src:  map[string]string{"intVal": "1"},
		want: &parseIntTest{IntVal: 1},
	}, {
		name: "int min value",
		src:  map[string]string{"intVal": fmt.Sprintf("%d", math.MinInt)},
		want: &parseIntTest{IntVal: math.MinInt},
	}, {
		name: "int max value",
		src:  map[string]string{"intVal": fmt.Sprintf("%d", math.MaxInt)},
		want: &parseIntTest{IntVal: math.MaxInt},
	}, {
		name:    "int min out of range value",
		src:     map[string]string{"intVal": "-9223372036854775809"},
		want:    &parseIntTest{},
		wantErr: api.NewParseError("intVal", "-9223372036854775809", strconv.ErrRange),
	}, {
		name:    "int max out of range value",
		src:     map[string]string{"intVal": "9223372036854775808"},
		want:    &parseIntTest{},
		wantErr: api.NewParseError("intVal", "9223372036854775808", strconv.ErrRange),
	}, {
		name:    "int syntax error",
		src:     map[string]string{"intVal": "one"},
		want:    &parseIntTest{},
		wantErr: api.NewParseError("intVal", "one", strconv.ErrSyntax),
	}, {
		name: "int8 zero value",
		src:  map[string]string{"int8Val": "0"},
		want: &parseInt8Test{},
	}, {
		name: "int8 in range value",
		src:  map[string]string{"int8Val": "10"},
		want: &parseInt8Test{Int8Val: 10},
	}, {
		name: "int8 min value",
		src:  map[string]string{"int8Val": "-128"},
		want: &parseInt8Test{Int8Val: math.MinInt8},
	}, {
		name: "int8 max value",
		src:  map[string]string{"int8Val": "127"},
		want: &parseInt8Test{Int8Val: math.MaxInt8},
	}, {
		name:    "int8 min out of range value",
		src:     map[string]string{"int8Val": "-129"},
		want:    &parseInt8Test{},
		wantErr: api.NewParseError("int8Val", "-129", strconv.ErrRange),
	}, {
		name:    "int8 max out of range value",
		src:     map[string]string{"int8Val": "128"},
		want:    &parseInt8Test{},
		wantErr: api.NewParseError("int8Val", "128", strconv.ErrRange),
	}, {
		name:    "int8 syntax error",
		src:     map[string]string{"int8Val": "ten"},
		want:    &parseInt8Test{},
		wantErr: api.NewParseError("int8Val", "ten", strconv.ErrSyntax),
	}, {
		name: "int16 zero value",
		src:  map[string]string{"int16Val": "0"},
		want: &parseInt16Test{},
	}, {
		name: "int16 in range value",
		src:  map[string]string{"int16Val": "100"},
		want: &parseInt16Test{Int16Val: 100},
	}, {
		name: "int16 min value",
		src:  map[string]string{"int16Val": "-32768"},
		want: &parseInt16Test{Int16Val: math.MinInt16},
	}, {
		name: "int16 max value",
		src:  map[string]string{"int16Val": "32767"},
		want: &parseInt16Test{Int16Val: math.MaxInt16},
	}, {
		name:    "int16 min out of range value",
		src:     map[string]string{"int16Val": "-32769"},
		want:    &parseInt16Test{},
		wantErr: api.NewParseError("int16Val", "-32769", strconv.ErrRange),
	}, {
		name:    "int16 max out of range value",
		src:     map[string]string{"int16Val": "32768"},
		want:    &parseInt16Test{},
		wantErr: api.NewParseError("int16Val", "32768", strconv.ErrRange),
	}, {
		name:    "int16 syntax error",
		src:     map[string]string{"int16Val": "hundred"},
		want:    &parseInt16Test{},
		wantErr: api.NewParseError("int16Val", "hundred", strconv.ErrSyntax),
	}, {
		name: "int32 zero value",
		src:  map[string]string{"int32Val": "0"},
		want: &parseInt32Test{},
	}, {
		name: "int32 in range value",
		src:  map[string]string{"int32Val": "1000"},
		want: &parseInt32Test{Int32Val: 1000},
	}, {
		name: "int32 min value",
		src:  map[string]string{"int32Val": "-2147483648"},
		want: &parseInt32Test{Int32Val: math.MinInt32},
	}, {
		name: "int32 max value",
		src:  map[string]string{"int32Val": "2147483647"},
		want: &parseInt32Test{Int32Val: math.MaxInt32},
	}, {
		name:    "int32 min out of range value",
		src:     map[string]string{"int32Val": "-2147483649"},
		want:    &parseInt32Test{},
		wantErr: api.NewParseError("int32Val", "-2147483649", strconv.ErrRange),
	}, {
		name:    "int32 max out of range value",
		src:     map[string]string{"int32Val": "2147483648"},
		want:    &parseInt32Test{},
		wantErr: api.NewParseError("int32Val", "2147483648", strconv.ErrRange),
	}, {
		name:    "int32 syntax error",
		src:     map[string]string{"int32Val": "thousand"},
		want:    &parseInt32Test{},
		wantErr: api.NewParseError("int32Val", "thousand", strconv.ErrSyntax),
	}, {
		name: "int64 zero value",
		src:  map[string]string{"int64Val": "0"},
		want: &parseInt64Test{},
	}, {
		name: "int64 in range value",
		src:  map[string]string{"int64Val": "10000"},
		want: &parseInt64Test{Int64Val: 10000},
	}, {
		name: "int64 min value",
		src:  map[string]string{"int64Val": "-9223372036854775808"},
		want: &parseInt64Test{Int64Val: math.MinInt64},
	}, {
		name: "int64 max value",
		src:  map[string]string{"int64Val": "9223372036854775807"},
		want: &parseInt64Test{Int64Val: math.MaxInt64},
	}, {
		name:    "int64 min out of range value",
		src:     map[string]string{"int64Val": "-9223372036854775809"},
		want:    &parseInt64Test{},
		wantErr: api.NewParseError("int64Val", "-9223372036854775809", strconv.ErrRange),
	}, {
		name:    "int64 max out of range value",
		src:     map[string]string{"int64Val": "9223372036854775808"},
		want:    &parseInt64Test{},
		wantErr: api.NewParseError("int64Val", "9223372036854775808", strconv.ErrRange),
	}, {
		name:    "int64 syntax error",
		src:     map[string]string{"int64Val": "ten thousand"},
		want:    &parseInt64Test{},
		wantErr: api.NewParseError("int64Val", "ten thousand", strconv.ErrSyntax),
	}, {
		name: "float32 zero value",
		src:  map[string]string{"float32Val": "0"},
		want: &parseFloat32Test{},
	}, {
		name: "float32 in range value",
		src:  map[string]string{"float32Val": "10.12345"},
		want: &parseFloat32Test{Float32Val: 10.12345},
	}, {
		name: "float32 min value",
		src:  map[string]string{"float32Val": "1.401298464324817070923729583289916131280e-45"},
		want: &parseFloat32Test{Float32Val: math.SmallestNonzeroFloat32},
	}, {
		name: "float32 max value",
		src:  map[string]string{"float32Val": "3.40282346638528859811704183484516925440e+38"},
		want: &parseFloat32Test{Float32Val: math.MaxFloat32},
	}, {
		name:    "float32 max out of range value",
		src:     map[string]string{"float32Val": "3.40282346638528859811704183484516925440e+39"},
		want:    &parseFloat32Test{},
		wantErr: api.NewParseError("float32Val", "3.40282346638528859811704183484516925440e+39", strconv.ErrRange),
	}, {
		name:    "float32 syntax error",
		src:     map[string]string{"float32Val": "ten point one ... five"},
		want:    &parseFloat32Test{},
		wantErr: api.NewParseError("float32Val", "ten point one ... five", strconv.ErrSyntax),
	}, {
		name: "float64 zero value",
		src:  map[string]string{"float64Val": "0"},
		want: &parseFloat64Test{},
	}, {
		name: "float64 in range value",
		src:  map[string]string{"float64Val": "10.123456789"},
		want: &parseFloat64Test{Float64Val: 10.123456789},
	}, {
		name: "float64 min value",
		src:  map[string]string{"float64Val": "4.9406564584124654417656879286822137236505980e-324"},
		want: &parseFloat64Test{Float64Val: math.SmallestNonzeroFloat64},
	}, {
		name: "float64 max value",
		src:  map[string]string{"float64Val": "1.79769313486231570814527423731704356798070e+308"},
		want: &parseFloat64Test{Float64Val: math.MaxFloat64},
	}, {
		name:    "float64 max out of range value",
		src:     map[string]string{"float64Val": "1.79769313486231570814527423731704356798070e+309"},
		want:    &parseFloat64Test{},
		wantErr: api.NewParseError("float64Val", "1.79769313486231570814527423731704356798070e+309", strconv.ErrRange),
	}, {
		name:    "float64 syntax error",
		src:     map[string]string{"float64Val": "ten point one ... nine"},
		want:    &parseFloat64Test{},
		wantErr: api.NewParseError("float64Val", "ten point one ... nine", strconv.ErrSyntax),
	}, {
		name: "byte slice zero value",
		src:  map[string]string{"byteSliceVal": ""},
		want: &parseByteSliceTest{},
	}, {
		name: "byte slice single byte",
		src:  map[string]string{"byteSliceVal": "66"},
		want: &parseByteSliceTest{ByteSliceVal: []byte{'f'}},
	}, {
		name: "byte slice multiple bytes",
		src:  map[string]string{"byteSliceVal": "000102030405060708090a0b0c0d0e0f"},
		want: &parseByteSliceTest{ByteSliceVal: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}},
	}, {
		name:    "byte slice invalid byte",
		src:     map[string]string{"byteSliceVal": "0g"},
		want:    &parseByteSliceTest{},
		wantErr: api.NewParseError("byteSliceVal", "0g", api.HexInvalidByteError('g')),
	}, {
		name:    "byte slice invalid length",
		src:     map[string]string{"byteSliceVal": "fff"},
		want:    &parseByteSliceTest{},
		wantErr: api.NewParseError("byteSliceVal", "fff", api.ErrHexLength),
	}, {
		name: "string zero value",
		src:  map[string]string{"stringVal": ""},
		want: &parseStringTest{},
	}, {
		name: "string single character",
		src:  map[string]string{"stringVal": "F"},
		want: &parseStringTest{StringVal: "F"},
	}, {
		name: "string multiple characters",
		src:  map[string]string{"stringVal": "Hello, World!"},
		want: &parseStringTest{StringVal: "Hello, World!"},
	}, {
		name: "string with multiple values",
		src:  map[string][]string{"stringVal": {"11", "22", "33"}},
		want: &parseStringTest{StringVal: "11"},
	}, {
		name: "string without matching field",
		src:  map[string]string{"-": "key does not match any field"},
		want: &parseStringTest{},
	}}
	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			have := reflect.New(reflect.TypeOf(tc.want).Elem()).Interface()
			haveErr := api.Parse(tc.src, have)
			if diff := cmp.Diff(tc.wantErr, haveErr, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("api.Parse(...): error mismatch (-want +have):\n%s", diff)
			}
			if diff := cmp.Diff(tc.want, have); diff != "" {
				t.Errorf("api.Parse(...): result mismatch (-want +have):\n%s", diff)
			}
		})
	}
}

func TestParse_InputOutputSanityCheck(t *testing.T) {
	t.Parallel()

	t.Run("input is nil", func(t *testing.T) {
		t.Parallel()

		var input interface{}
		err := api.Parse(input, struct{}{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("input is not a map", func(t *testing.T) {
		t.Parallel()

		input := "foo"
		err := api.Parse(&input, struct{}{})
		if err == nil {
			t.Fatalf("expected error; have none")
		}
	})

	t.Run("output is not a pointer", func(t *testing.T) {
		t.Parallel()

		var (
			input  = map[string]interface{}{"someVal": "123"}
			output struct {
				SomeVal string `parse:"someVal"`
			}
		)
		err := api.Parse(&input, output)
		if err == nil {
			t.Fatalf("expected error; have none")
		}
	})

	t.Run("output is nil", func(t *testing.T) {
		t.Parallel()

		var (
			input  = map[string]interface{}{"someVal": "123"}
			output interface{}
		)
		err := api.Parse(&input, output)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("output is a nil pointer", func(t *testing.T) {
		t.Parallel()

		var (
			input  = map[string]interface{}{"someVal": "123"}
			output = struct {
				SomeVal string `parse:"someVal"`
			}{}
		)
		err := api.Parse(&input, &output)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("output is not a struct", func(t *testing.T) {
		t.Parallel()

		var (
			input  = map[string]interface{}{"someVal": "123"}
			output = "foo"
		)
		err := api.Parse(&input, &output)
		if err == nil {
			t.Fatalf("expected error; have none")
		}
	})
}
