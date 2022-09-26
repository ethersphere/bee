// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/hashicorp/go-multierror"
)

// parseTagName represents the name of the tag used to parse the value.
const parseTagName = "parse"

// errHexLength reports an attempt to decode an odd-length input.
// It's a drop-in replacement for hex.ErrLength.
var errHexLength = errors.New("odd length hex string")

// hexInvalidByteError values describe errors resulting
// from an invalid byte in a hex string.
// It's a drop-in replacement for hex.InvalidByteError.
type hexInvalidByteError byte

func (e hexInvalidByteError) Error() string {
	return fmt.Sprintf("invalid byte: %#U", rune(e))
}

// parseError is returned when a parameter cannot be parsed.
type parseError struct {
	Param string
	Value string
	Cause error
}

// Error implements the error interface.
func (e parseError) Error() string {
	return fmt.Sprintf("`%s=%v`: %v", e.Param, e.Value, e.Cause)
}

// Unwrap implements the interface required by errors.Unwrap function.
func (e parseError) Unwrap() error {
	return e.Cause
}

// newParseError returns a new parse error.
// If the cause is strconv.NumError, its
// underlying error is unwrapped and
// used as a cause. The hex.InvalidByteError
// and hex.ErrLength errors are replaced in
// order to hide unnecessary information.
func newParseError(param, value string, cause error) error {
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

	return parseError{
		Param: param,
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

// parse parses the input and sets the output values.
// The input is on of the following:
//   - map[string]string
//   - map[string][]string
//
// In the second case, the first value of
// the string array is taken as a value.
//
// The output is a struct with fields of type:
//   - bool
//   - uint, uint8, uint16, uint32, uint64
//   - int, int8, int16, int32, int64
//   - float32, float64
//   - []byte
//   - string
//
// The output struct fields can contain the
// `parse` tag that refers to the map input key.
// For example:
//
//	type Output struct {
//		BoolVal bool `parse:"boolVal"`
//	}
//
// If the `parse` tag is not present, the field name is used.
// If the field name or the `parse` tag is not present in
// the input map, the field is skipped.
//
// In case of parsing error, a new parseError is returned to the caller.
// The caller can use the Unwrap method to get the original error.
func parse(input, output interface{}) error {
	if input == nil || output == nil {
		return nil
	}

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

	// Parse input into output.
	pErrs := &multierror.Error{ErrorFormat: flattenErrorsFormat}
	for i := 0; i < outputVal.NumField(); i++ {
		param := outputVal.Type().Field(i).Name
		val, ok := outputVal.Type().Field(i).Tag.Lookup(parseTagName)
		if ok {
			param = val
		}

		mKey := reflect.ValueOf(param)
		mVal := inputVal.MapIndex(mKey)
		if !mVal.IsValid() {
			continue
		}
		value := flattenValue(mVal).String()

		switch fieldKind := outputVal.Field(i).Kind(); fieldKind {
		case reflect.Bool:
			val, err := strconv.ParseBool(value)
			if err != nil {
				pErrs = multierror.Append(pErrs, newParseError(param, value, err))
				continue
			}
			outputVal.Field(i).SetBool(val)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			val, err := strconv.ParseUint(value, 10, numberSize(fieldKind))
			if err != nil {
				pErrs = multierror.Append(pErrs, newParseError(param, value, err))
				continue
			}
			outputVal.Field(i).SetUint(val)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val, err := strconv.ParseInt(value, 10, numberSize(fieldKind))
			if err != nil {
				pErrs = multierror.Append(pErrs, newParseError(param, value, err))
				continue
			}
			outputVal.Field(i).SetInt(val)
		case reflect.Float32, reflect.Float64:
			val, err := strconv.ParseFloat(value, numberSize(fieldKind))
			if err != nil {
				pErrs = multierror.Append(pErrs, newParseError(param, value, err))
				continue
			}
			outputVal.Field(i).SetFloat(val)
		case reflect.Slice:
			if value == "" {
				continue // Nil slice.
			}
			val, err := hex.DecodeString(value)
			if err != nil {
				pErrs = multierror.Append(pErrs, newParseError(param, value, err))
				continue
			}
			outputVal.Field(i).SetBytes(val)
		case reflect.String:
			outputVal.Field(i).SetString(value)
		default:
			return fmt.Errorf("unsupported type %T", outputVal.Field(i).Interface())
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
