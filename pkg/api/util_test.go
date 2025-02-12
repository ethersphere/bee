// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

type (
	mapBoolTest struct {
		BoolVal bool `map:"boolVal"`
	}

	mapUintTest struct {
		UintVal uint `map:"uintVal"`
	}

	mapUint8Test struct {
		Uint8Val uint8 `map:"uint8Val"`
	}

	mapUint16Test struct {
		Uint16Val uint16 `map:"uint16Val"`
	}

	mapUint32Test struct {
		Uint32Val uint32 `map:"uint32Val"`
	}

	mapUint64Test struct {
		Uint64Val uint64 `map:"uint64Val"`
	}

	mapIntTest struct {
		IntVal int `map:"intVal"`
	}

	mapInt8Test struct {
		Int8Val int8 `map:"int8Val"`
	}

	mapInt16Test struct {
		Int16Val int16 `map:"int16Val"`
	}

	mapInt32Test struct {
		Int32Val int32 `map:"int32Val"`
	}

	mapInt64Test struct {
		Int64Val int64 `map:"int64Val"`
	}

	mapFloat32Test struct {
		Float32Val float32 `map:"float32Val"`
	}

	mapFloat64Test struct {
		Float64Val float64 `map:"float64Val"`
	}

	mapByteSliceTest struct {
		ByteSliceVal []byte `map:"byteSliceVal"`
	}

	mapStringTest struct {
		StringVal string `map:"stringVal"`
	}

	mapStringWithOmitemptyTest struct {
		StringVal string `map:"stringVal,omitempty"`
	}

	mapBigIntTest struct {
		BigIntVal *big.Int `map:"bigIntVal"`
	}

	mapCommonHashTest struct {
		CommonHashVal common.Hash `map:"commonHashVal"`
	}

	mapSwarmAddressTest struct {
		SwarmAddressVal swarm.Address `map:"swarmAddressVal"`
	}
)

func TestMapStructure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     interface{}
		want    interface{}
		wantErr error
	}{{
		name: "bool zero value",
		src:  map[string]string{"boolVal": "0"},
		want: &mapBoolTest{},
	}, {
		name: "bool false",
		src:  map[string]string{"boolVal": "false"},
		want: &mapBoolTest{BoolVal: false},
	}, {
		name: "bool true",
		src:  map[string]string{"boolVal": "true"},
		want: &mapBoolTest{BoolVal: true},
	}, {
		name:    "bool syntax error",
		src:     map[string]string{"boolVal": "a"},
		want:    &mapBoolTest{},
		wantErr: api.NewParseError("boolVal", "a", strconv.ErrSyntax),
	}, {
		name: "uint zero value",
		src:  map[string]string{"uintVal": "0"},
		want: &mapUintTest{},
	}, {
		name: "uint in range value",
		src:  map[string]string{"uintVal": "1"},
		want: &mapUintTest{UintVal: 1},
	}, {
		name: "uint max value",
		src:  map[string]string{"uintVal": fmt.Sprintf("%d", uint(math.MaxUint))},
		want: &mapUintTest{UintVal: math.MaxUint},
	}, {
		name:    "uint out of range value",
		src:     map[string]string{"uintVal": "18446744073709551616"},
		want:    &mapUintTest{},
		wantErr: api.NewParseError("uintVal", "18446744073709551616", strconv.ErrRange),
	}, {
		name:    "uint syntax error",
		src:     map[string]string{"uintVal": "one"},
		want:    &mapUintTest{},
		wantErr: api.NewParseError("uintVal", "one", strconv.ErrSyntax),
	}, {
		name: "uint8 zero value",
		src:  map[string]string{"uint8Val": "0"},
		want: &mapUint8Test{},
	}, {
		name: "uint8 in range value",
		src:  map[string]string{"uint8Val": "10"},
		want: &mapUint8Test{Uint8Val: 10},
	}, {
		name: "uint8 max value",
		src:  map[string]string{"uint8Val": "255"},
		want: &mapUint8Test{Uint8Val: math.MaxUint8},
	}, {
		name:    "uint8 out of range value",
		src:     map[string]string{"uint8Val": "256"},
		want:    &mapUint8Test{},
		wantErr: api.NewParseError("uint8Val", "256", strconv.ErrRange),
	}, {
		name:    "uint8 syntax error",
		src:     map[string]string{"uint8Val": "ten"},
		want:    &mapUint8Test{},
		wantErr: api.NewParseError("uint8Val", "ten", strconv.ErrSyntax),
	}, {
		name: "uint16 zero value",
		src:  map[string]string{"uint16Val": "0"},
		want: &mapUint16Test{},
	}, {
		name: "uint16 in range value",
		src:  map[string]string{"uint16Val": "100"},
		want: &mapUint16Test{Uint16Val: 100},
	}, {
		name: "uint16 max value",
		src:  map[string]string{"uint16Val": "65535"},
		want: &mapUint16Test{Uint16Val: math.MaxUint16},
	}, {
		name:    "uint16 out of range value",
		src:     map[string]string{"uint16Val": "65536"},
		want:    &mapUint16Test{},
		wantErr: api.NewParseError("uint16Val", "65536", strconv.ErrRange),
	}, {
		name:    "uint16 syntax error",
		src:     map[string]string{"uint16Val": "hundred"},
		want:    &mapUint16Test{},
		wantErr: api.NewParseError("uint16Val", "hundred", strconv.ErrSyntax),
	}, {
		name: "uint32 zero value",
		src:  map[string]string{"uint32Val": "0"},
		want: &mapUint32Test{},
	}, {
		name: "uint32 in range value",
		src:  map[string]string{"uint32Val": "1000"},
		want: &mapUint32Test{Uint32Val: 1000},
	}, {
		name: "uint32 max value",
		src:  map[string]string{"uint32Val": "4294967295"},
		want: &mapUint32Test{Uint32Val: math.MaxUint32},
	}, {
		name:    "uint32 out of range value",
		src:     map[string]string{"uint32Val": "4294967296"},
		want:    &mapUint32Test{},
		wantErr: api.NewParseError("uint32Val", "4294967296", strconv.ErrRange),
	}, {
		name:    "uint32 syntax error",
		src:     map[string]string{"uint32Val": "thousand"},
		want:    &mapUint32Test{},
		wantErr: api.NewParseError("uint32Val", "thousand", strconv.ErrSyntax),
	}, {
		name: "uint64 zero value",
		src:  map[string]string{"uint64Val": "0"},
		want: &mapUint64Test{},
	}, {
		name: "uint64 in range value",
		src:  map[string]string{"uint64Val": "10000"},
		want: &mapUint64Test{Uint64Val: 10000},
	}, {
		name: "uint64 max value",
		src:  map[string]string{"uint64Val": "18446744073709551615"},
		want: &mapUint64Test{Uint64Val: math.MaxUint64},
	}, {
		name:    "uint64 out of range value",
		src:     map[string]string{"uint64Val": "18446744073709551616"},
		want:    &mapUint64Test{},
		wantErr: api.NewParseError("uint64Val", "18446744073709551616", strconv.ErrRange),
	}, {
		name:    "uint64 syntax error",
		src:     map[string]string{"uint64Val": "ten thousand"},
		want:    &mapUint64Test{},
		wantErr: api.NewParseError("uint64Val", "ten thousand", strconv.ErrSyntax),
	}, {
		name: "int zero value",
		src:  map[string]string{"intVal": "0"},
		want: &mapIntTest{},
	}, {
		name: "int in range value",
		src:  map[string]string{"intVal": "1"},
		want: &mapIntTest{IntVal: 1},
	}, {
		name: "int min value",
		src:  map[string]string{"intVal": fmt.Sprintf("%d", math.MinInt)},
		want: &mapIntTest{IntVal: math.MinInt},
	}, {
		name: "int max value",
		src:  map[string]string{"intVal": fmt.Sprintf("%d", math.MaxInt)},
		want: &mapIntTest{IntVal: math.MaxInt},
	}, {
		name:    "int min out of range value",
		src:     map[string]string{"intVal": "-9223372036854775809"},
		want:    &mapIntTest{},
		wantErr: api.NewParseError("intVal", "-9223372036854775809", strconv.ErrRange),
	}, {
		name:    "int max out of range value",
		src:     map[string]string{"intVal": "9223372036854775808"},
		want:    &mapIntTest{},
		wantErr: api.NewParseError("intVal", "9223372036854775808", strconv.ErrRange),
	}, {
		name:    "int syntax error",
		src:     map[string]string{"intVal": "one"},
		want:    &mapIntTest{},
		wantErr: api.NewParseError("intVal", "one", strconv.ErrSyntax),
	}, {
		name: "int8 zero value",
		src:  map[string]string{"int8Val": "0"},
		want: &mapInt8Test{},
	}, {
		name: "int8 in range value",
		src:  map[string]string{"int8Val": "10"},
		want: &mapInt8Test{Int8Val: 10},
	}, {
		name: "int8 min value",
		src:  map[string]string{"int8Val": "-128"},
		want: &mapInt8Test{Int8Val: math.MinInt8},
	}, {
		name: "int8 max value",
		src:  map[string]string{"int8Val": "127"},
		want: &mapInt8Test{Int8Val: math.MaxInt8},
	}, {
		name:    "int8 min out of range value",
		src:     map[string]string{"int8Val": "-129"},
		want:    &mapInt8Test{},
		wantErr: api.NewParseError("int8Val", "-129", strconv.ErrRange),
	}, {
		name:    "int8 max out of range value",
		src:     map[string]string{"int8Val": "128"},
		want:    &mapInt8Test{},
		wantErr: api.NewParseError("int8Val", "128", strconv.ErrRange),
	}, {
		name:    "int8 syntax error",
		src:     map[string]string{"int8Val": "ten"},
		want:    &mapInt8Test{},
		wantErr: api.NewParseError("int8Val", "ten", strconv.ErrSyntax),
	}, {
		name: "int16 zero value",
		src:  map[string]string{"int16Val": "0"},
		want: &mapInt16Test{},
	}, {
		name: "int16 in range value",
		src:  map[string]string{"int16Val": "100"},
		want: &mapInt16Test{Int16Val: 100},
	}, {
		name: "int16 min value",
		src:  map[string]string{"int16Val": "-32768"},
		want: &mapInt16Test{Int16Val: math.MinInt16},
	}, {
		name: "int16 max value",
		src:  map[string]string{"int16Val": "32767"},
		want: &mapInt16Test{Int16Val: math.MaxInt16},
	}, {
		name:    "int16 min out of range value",
		src:     map[string]string{"int16Val": "-32769"},
		want:    &mapInt16Test{},
		wantErr: api.NewParseError("int16Val", "-32769", strconv.ErrRange),
	}, {
		name:    "int16 max out of range value",
		src:     map[string]string{"int16Val": "32768"},
		want:    &mapInt16Test{},
		wantErr: api.NewParseError("int16Val", "32768", strconv.ErrRange),
	}, {
		name:    "int16 syntax error",
		src:     map[string]string{"int16Val": "hundred"},
		want:    &mapInt16Test{},
		wantErr: api.NewParseError("int16Val", "hundred", strconv.ErrSyntax),
	}, {
		name: "int32 zero value",
		src:  map[string]string{"int32Val": "0"},
		want: &mapInt32Test{},
	}, {
		name: "int32 in range value",
		src:  map[string]string{"int32Val": "1000"},
		want: &mapInt32Test{Int32Val: 1000},
	}, {
		name: "int32 min value",
		src:  map[string]string{"int32Val": "-2147483648"},
		want: &mapInt32Test{Int32Val: math.MinInt32},
	}, {
		name: "int32 max value",
		src:  map[string]string{"int32Val": "2147483647"},
		want: &mapInt32Test{Int32Val: math.MaxInt32},
	}, {
		name:    "int32 min out of range value",
		src:     map[string]string{"int32Val": "-2147483649"},
		want:    &mapInt32Test{},
		wantErr: api.NewParseError("int32Val", "-2147483649", strconv.ErrRange),
	}, {
		name:    "int32 max out of range value",
		src:     map[string]string{"int32Val": "2147483648"},
		want:    &mapInt32Test{},
		wantErr: api.NewParseError("int32Val", "2147483648", strconv.ErrRange),
	}, {
		name:    "int32 syntax error",
		src:     map[string]string{"int32Val": "thousand"},
		want:    &mapInt32Test{},
		wantErr: api.NewParseError("int32Val", "thousand", strconv.ErrSyntax),
	}, {
		name: "int64 zero value",
		src:  map[string]string{"int64Val": "0"},
		want: &mapInt64Test{},
	}, {
		name: "int64 in range value",
		src:  map[string]string{"int64Val": "10000"},
		want: &mapInt64Test{Int64Val: 10000},
	}, {
		name: "int64 min value",
		src:  map[string]string{"int64Val": "-9223372036854775808"},
		want: &mapInt64Test{Int64Val: math.MinInt64},
	}, {
		name: "int64 max value",
		src:  map[string]string{"int64Val": "9223372036854775807"},
		want: &mapInt64Test{Int64Val: math.MaxInt64},
	}, {
		name:    "int64 min out of range value",
		src:     map[string]string{"int64Val": "-9223372036854775809"},
		want:    &mapInt64Test{},
		wantErr: api.NewParseError("int64Val", "-9223372036854775809", strconv.ErrRange),
	}, {
		name:    "int64 max out of range value",
		src:     map[string]string{"int64Val": "9223372036854775808"},
		want:    &mapInt64Test{},
		wantErr: api.NewParseError("int64Val", "9223372036854775808", strconv.ErrRange),
	}, {
		name:    "int64 syntax error",
		src:     map[string]string{"int64Val": "ten thousand"},
		want:    &mapInt64Test{},
		wantErr: api.NewParseError("int64Val", "ten thousand", strconv.ErrSyntax),
	}, {
		name: "float32 zero value",
		src:  map[string]string{"float32Val": "0"},
		want: &mapFloat32Test{},
	}, {
		name: "float32 in range value",
		src:  map[string]string{"float32Val": "10.12345"},
		want: &mapFloat32Test{Float32Val: 10.12345},
	}, {
		name: "float32 min value",
		src:  map[string]string{"float32Val": "1.401298464324817070923729583289916131280e-45"},
		want: &mapFloat32Test{Float32Val: math.SmallestNonzeroFloat32},
	}, {
		name: "float32 max value",
		src:  map[string]string{"float32Val": "3.40282346638528859811704183484516925440e+38"},
		want: &mapFloat32Test{Float32Val: math.MaxFloat32},
	}, {
		name:    "float32 max out of range value",
		src:     map[string]string{"float32Val": "3.40282346638528859811704183484516925440e+39"},
		want:    &mapFloat32Test{},
		wantErr: api.NewParseError("float32Val", "3.40282346638528859811704183484516925440e+39", strconv.ErrRange),
	}, {
		name:    "float32 syntax error",
		src:     map[string]string{"float32Val": "ten point one ... five"},
		want:    &mapFloat32Test{},
		wantErr: api.NewParseError("float32Val", "ten point one ... five", strconv.ErrSyntax),
	}, {
		name: "float64 zero value",
		src:  map[string]string{"float64Val": "0"},
		want: &mapFloat64Test{},
	}, {
		name: "float64 in range value",
		src:  map[string]string{"float64Val": "10.123456789"},
		want: &mapFloat64Test{Float64Val: 10.123456789},
	}, {
		name: "float64 min value",
		src:  map[string]string{"float64Val": "4.9406564584124654417656879286822137236505980e-324"},
		want: &mapFloat64Test{Float64Val: math.SmallestNonzeroFloat64},
	}, {
		name: "float64 max value",
		src:  map[string]string{"float64Val": "1.79769313486231570814527423731704356798070e+308"},
		want: &mapFloat64Test{Float64Val: math.MaxFloat64},
	}, {
		name:    "float64 max out of range value",
		src:     map[string]string{"float64Val": "1.79769313486231570814527423731704356798070e+309"},
		want:    &mapFloat64Test{},
		wantErr: api.NewParseError("float64Val", "1.79769313486231570814527423731704356798070e+309", strconv.ErrRange),
	}, {
		name:    "float64 syntax error",
		src:     map[string]string{"float64Val": "ten point one ... nine"},
		want:    &mapFloat64Test{},
		wantErr: api.NewParseError("float64Val", "ten point one ... nine", strconv.ErrSyntax),
	}, {
		name: "byte slice zero value",
		src:  map[string]string{"byteSliceVal": ""},
		want: &mapByteSliceTest{},
	}, {
		name: "byte slice single byte",
		src:  map[string]string{"byteSliceVal": "66"},
		want: &mapByteSliceTest{ByteSliceVal: []byte{'f'}},
	}, {
		name: "byte slice multiple bytes",
		src:  map[string]string{"byteSliceVal": "000102030405060708090a0b0c0d0e0f"},
		want: &mapByteSliceTest{ByteSliceVal: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}},
	}, {
		name:    "byte slice invalid byte",
		src:     map[string]string{"byteSliceVal": "0g"},
		want:    &mapByteSliceTest{},
		wantErr: api.NewParseError("byteSliceVal", "0g", api.HexInvalidByteError('g')),
	}, {
		name:    "byte slice invalid length",
		src:     map[string]string{"byteSliceVal": "fff"},
		want:    &mapByteSliceTest{},
		wantErr: api.NewParseError("byteSliceVal", "fff", api.ErrHexLength),
	}, {
		name: "string zero value",
		src:  map[string]string{"stringVal": ""},
		want: &mapStringTest{},
	}, {
		name: "string single character",
		src:  map[string]string{"stringVal": "F"},
		want: &mapStringTest{StringVal: "F"},
	}, {
		name: "string multiple characters",
		src:  map[string]string{"stringVal": "Hello, World!"},
		want: &mapStringTest{StringVal: "Hello, World!"},
	}, {
		name: "string with multiple values",
		src:  map[string][]string{"stringVal": {"11", "22", "33"}},
		want: &mapStringTest{StringVal: "11"},
	}, {
		name: "string without matching field",
		src:  map[string]string{"-": "key does not match any field"},
		want: &mapStringTest{},
	}, {
		name: "string with omitempty field",
		src:  map[string]string{"stringVal": ""},
		want: &mapStringWithOmitemptyTest{},
	}, {
		name: "bit.Int value",
		src:  map[string]string{"bigIntVal": "1234567890"},
		want: &mapBigIntTest{BigIntVal: new(big.Int).SetInt64(1234567890)},
	}, {
		name: "common.Hash value",
		src:  map[string]string{"commonHashVal": "0x1234567890abcdef"},
		want: &mapCommonHashTest{CommonHashVal: common.HexToHash("0x1234567890abcdef")},
	}, {
		name: "swarm.Address value",
		src:  map[string]string{"swarmAddressVal": "1234567890abcdef"},
		want: &mapSwarmAddressTest{SwarmAddressVal: swarm.MustParseHexAddress("1234567890abcdef")},
	}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			have := reflect.New(reflect.TypeOf(tc.want).Elem()).Interface()
			haveErr := errors.Unwrap(api.MapStructure(tc.src, have, nil))
			if diff := cmp.Diff(tc.wantErr, haveErr); diff != "" {
				t.Fatalf("api.mapStructure(...): error mismatch (-want +have):\n%s", diff)
			}
			if diff := cmp.Diff(tc.want, have, cmp.AllowUnexported(big.Int{})); diff != "" {
				t.Errorf("api.mapStructure(...): result mismatch (-want +have):\n%s", diff)
			}
		})
	}
}

func TestMapStructure_InputOutputSanityCheck(t *testing.T) {
	t.Parallel()

	t.Run("input is nil", func(t *testing.T) {
		t.Parallel()

		var input interface{}
		err := api.MapStructure(input, struct{}{}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("input is not a map", func(t *testing.T) {
		t.Parallel()

		input := "foo"
		err := api.MapStructure(&input, struct{}{}, nil)
		if err == nil {
			t.Fatalf("expected error; have none")
		}
	})

	t.Run("output is not a pointer", func(t *testing.T) {
		t.Parallel()

		var (
			input  = map[string]interface{}{"someVal": "123"}
			output struct {
				SomeVal string `map:"someVal"`
			}
		)
		err := api.MapStructure(&input, output, nil)
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
		err := api.MapStructure(&input, output, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("output is a nil pointer", func(t *testing.T) {
		t.Parallel()

		var (
			input  = map[string]interface{}{"someVal": "123"}
			output = struct {
				SomeVal string `map:"someVal"`
			}{}
		)
		err := api.MapStructure(&input, &output, nil)
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
		err := api.MapStructure(&input, &output, nil)
		if err == nil {
			t.Fatalf("expected error; have none")
		}
	})
}
