// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/pss"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/hashicorp/go-multierror"
	"github.com/multiformats/go-multiaddr"
)

// mapStructureTagName represents the name of the tag used to map values.
const mapStructureTagName = "map"

// errHexLength reports an attempt to decode an odd-length input.
// It's a drop-in replacement for hex.ErrLength.
var errHexLength = errors.New("odd length hex string")

// hexInvalidByteError values describe errors resulting
// from an invalid byte in a hex string.
// It's a drop-in replacement for hex.InvalidByteError.
type hexInvalidByteError byte

// Error implements the error interface.
func (e hexInvalidByteError) Error() string {
	return fmt.Sprintf("invalid hex byte: %#U", rune(e))
}

// parseError is returned when an entry cannot be parsed.
type parseError struct {
	Entry string
	Value string
	Cause error
}

// Error implements the error interface.
func (e *parseError) Error() string {
	return fmt.Sprintf("`%s=%v`: %v", e.Entry, e.Value, e.Cause)
}

// Unwrap implements the interface required by errors.Unwrap function.
func (e *parseError) Unwrap() error {
	return e.Cause
}

// Equal returns true if the given error
// type and fields are equal to this error.
// It is used to compare errors in tests.
func (e *parseError) Equal(err error) bool {
	var p *parseError
	if !errors.As(err, &p) {
		return false
	}
	return e.Entry == p.Entry && e.Value == p.Value && errors.Is(e.Cause, p.Cause)
}

// newParseError returns a new mapStructure error.
// If the cause is strconv.NumError, its
// underlying error is unwrapped and
// used as a cause. The hex.InvalidByteError
// and hex.ErrLength errors are replaced in
// order to hide unnecessary information.
func newParseError(entry, value string, cause error) error {
	var numErr *strconv.NumError
	if errors.As(cause, &numErr) {
		cause = numErr.Err
	}

	var hexErr hex.InvalidByteError
	if errors.As(cause, &hexErr) {
		cause = hexInvalidByteError(hexErr)
	}

	if errors.Is(cause, hex.ErrLength) {
		cause = errHexLength
	}

	return &parseError{
		Entry: entry,
		Value: value,
		Cause: cause,
	}
}

// flattenErrorsFormat flattens the errors in
// the multierror.Error as a one-line string.
var flattenErrorsFormat = func(es []error) string {
	messages := make([]string, len(es))
	for i, err := range es {
		messages[i] = err.Error()
	}
	return fmt.Sprintf(
		"%d error(s) occurred: %v",
		len(es),
		strings.Join(messages, "; "),
	)
}

// mapStructure maps the input to the output values.
// The input is one of the following:
//   - map[string]string
//   - map[string][]string
//
// In the second case, the first value of
// the string array is taken as a value.
//
// The output struct fields can contain the
// `map` tag that refers to the map input key.
// For example:
//
//	type Output struct {
//		BoolVal bool `map:"boolVal,omitempty"`
//	}
//
// If the `map` tag is not present, the field name is used.
// If the field name or the `map` tag is not present in
// the input map, the field is skipped. If the map value
// is empty and the` omitempty` tag is present then the
// field is skipped.
//
// In case of parsing error, a new parseError is returned to the caller.
// The caller can use the Unwrap method to get the original error.
func mapStructure(input, output interface{}, hooks map[string]func(v string) (string, error)) (err error) {
	if input == nil || output == nil {
		return nil
	}

	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("%v", e)
		}
	}()

	var (
		inputVal  reflect.Value
		outputVal reflect.Value
	)

	// Do input sanity checks.
	inputVal = reflect.ValueOf(input)
	if inputVal.Kind() == reflect.Ptr {
		inputVal = inputVal.Elem()
	}
	switch {
	case inputVal.Kind() != reflect.Map:
		return errors.New("input is not a map")
	case !inputVal.IsValid():
		return nil
	}

	// Do output sanity checks.
	outputVal = reflect.ValueOf(output)
	switch {
	case outputVal.Kind() != reflect.Ptr:
		return errors.New("output is not a pointer")
	case outputVal.Elem().Kind() != reflect.Struct:
		return errors.New("output is not a struct")
	}
	outputVal = outputVal.Elem()

	// set is the workhorse here, parsing and setting the values.
	var set func(string, reflect.Value) error
	set = func(value string, field reflect.Value) error {
		switch fieldKind := field.Kind(); fieldKind {
		case reflect.Ptr:
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			err := set(value, field.Elem())
			if err != nil {
				field.Set(reflect.Zero(field.Type())) // Clear the field on error.
			}
			return err
		case reflect.Bool:
			val, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			field.SetBool(val)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			val, err := strconv.ParseUint(value, 10, numberSize(fieldKind))
			if err != nil {
				return err
			}
			field.SetUint(val)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val, err := strconv.ParseInt(value, 10, numberSize(fieldKind))
			if err != nil {
				return err
			}
			field.SetInt(val)
		case reflect.Float32, reflect.Float64:
			val, err := strconv.ParseFloat(value, numberSize(fieldKind))
			if err != nil {
				return err
			}
			field.SetFloat(val)
		case reflect.String:
			field.SetString(value)
		case reflect.Slice:
			if value == "" {
				return nil // Nil slice.
			}
			val, err := hex.DecodeString(value)
			if err != nil {
				return err
			}
			field.SetBytes(val)
		case reflect.Array:
			switch field.Interface().(type) {
			case common.Hash:
				val := common.HexToHash(value)
				field.Set(reflect.ValueOf(val))
			case common.Address:
				val := common.HexToAddress(value)
				field.Set(reflect.ValueOf(val))
			}
		case reflect.Struct:
			switch field.Interface().(type) {
			case big.Int:
				val, ok := new(big.Int).SetString(value, 10)
				if !ok {
					return errors.New("invalid value")
				}
				field.Set(reflect.ValueOf(*val))
			case swarm.Address:
				val, err := swarm.ParseHexAddress(value)
				if err != nil {
					return err
				}
				field.Set(reflect.ValueOf(val))
			case common.Hash:
				val := common.HexToHash(value)
				field.Set(reflect.ValueOf(val))
			case ecdsa.PublicKey:
				val, err := pss.ParseRecipient(value)
				if err != nil {
					return err
				}
				field.Set(reflect.ValueOf(*val))
			}
		case reflect.Interface:
			switch field.Type() {
			case reflect.TypeOf((*multiaddr.Multiaddr)(nil)).Elem():
				val, err := multiaddr.NewMultiaddr(value)
				if err != nil {
					return err
				}
				field.Set(reflect.ValueOf(val))
			}
		default:
			return fmt.Errorf("unsupported type %T", field.Interface())
		}
		return nil
	}

	// parseFieldTags parses the given field tags into name, hook, and omitempty.
	parseFieldTags := func(field reflect.StructField) (name string, hook func(v string) (string, error), omitempty bool) {
		hook = func(v string) (string, error) { return v, nil }

		val, ok := field.Tag.Lookup(mapStructureTagName)
		if !ok {
			return field.Name, hook, false
		}

		tags := strings.SplitN(val, ",", 3)
		name = tags[0]
		for _, tag := range tags[1:] {
			switch tag {
			case "omitempty":
				omitempty = true
			default:
				if len(hooks) == 0 {
					panic(errors.New("zero registered hooks"))
				}
				hook, ok = hooks[tag]
				if !ok {
					panic(fmt.Errorf("unknown hook %q for field: %s", tag, field.Name))
				}
			}
		}

		return name, hook, omitempty
	}

	// Map input into output.
	pErrs := &multierror.Error{ErrorFormat: flattenErrorsFormat}
	for i := 0; i < outputVal.NumField(); i++ {
		name, hook, omitempty := parseFieldTags(outputVal.Type().Field(i))

		mKey := reflect.ValueOf(name)
		mVal := inputVal.MapIndex(mKey)
		if !mVal.IsValid() {
			continue
		}

		value := flattenValue(mVal).String()
		if omitempty && value == "" {
			continue
		}

		trans, err := hook(value)
		if err != nil {
			pErrs = multierror.Append(pErrs, newParseError(name, value, err))
			continue
		}

		if err := set(trans, outputVal.Field(i)); err != nil {
			pErrs = multierror.Append(pErrs, newParseError(name, value, err))
		}
	}
	return pErrs.ErrorOrNil()
}

// numberSize returns the size of the number in bits.
func numberSize(k reflect.Kind) int {
	switch k {
	case reflect.Uint8, reflect.Int8:
		return 8
	case reflect.Uint16, reflect.Int16:
		return 16
	case reflect.Uint32, reflect.Int32, reflect.Float32:
		return 32
	case reflect.Uint64, reflect.Int64, reflect.Float64:
		return 64
	}
	return 0
}

// flattenValue returns the first element of the value if it is a slice.
func flattenValue(val reflect.Value) reflect.Value {
	switch val.Kind() {
	case reflect.Slice:
		return val.Index(0)
	}
	return val
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}
