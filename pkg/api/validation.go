// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

type ValidateFunc map[string]func(string, reflect.Value) error

var parseHooks = ValidateFunc{
	"hexStringToBytes": parseBatchId,
	"addressToBytes":   parseAddress,
	"hexToHash":        parseHexToHash,
}

// parseAndValidate parses the input and validates it
// against the annotations declared in the given struct.
func (s *Service) parseAndValidate(input, output interface{}) (err error) {
	err = parse(input, output)
	if err != nil {
		return err
	}

	return nil
}

func parse(input, output interface{}) (err error) {
	val := reflect.ValueOf(output).Elem()

	reqMapVars := make(map[string]string)
	reqMapQuery := make(map[string][]string)

	switch v := input.(type) {
	case map[string]string:
		reqMapVars = v
	case url.Values:
		reqMapQuery = v
	case *http.Request:
		reqMapVars = mux.Vars(v)
		reqMapQuery = v.URL.Query()
	}

	for i := 0; i < val.NumField(); i++ {
		parseProperty := val.Type().Field(i).Tag.Get("parse")
		res := strings.FieldsFunc(parseProperty, func(r rune) bool {
			return r == ',' || r == ' '
		})
		if len(res) == 0 {
			return errors.New("invalid parse tag")
		}
		reqName := res[0]
		customHook := ""
		if len(res) == 2 {
			customHook = res[1]
		}

		errMessage := val.Type().Field(i).Tag.Get("errMessage")

		var reqValue string
		if varValue, isExist := reqMapVars[reqName]; isExist {
			reqValue = varValue
		}
		if queryValue := reqMapQuery[reqName]; len(queryValue) > 0 {
			reqValue = queryValue[0]
		}
		if len(customHook) > 0 {
			err = parseHooks[customHook](reqValue, val.Field(i))
			if err != nil {
				return GetErrorMessage(reqName, errMessage)
			}
		} else {
			switch val.Type().Field(i).Type.Kind() {
			case reflect.Uint8:
				err = decodeUint8(reqValue, val.Field(i))
			case reflect.Uint16:
				err = decodeUint16(reqValue, val.Field(i))
			case reflect.Uint32:
				err = decodeUint32(reqValue, val.Field(i))
			case reflect.Uint64:
				err = decodeUint64(reqValue, val.Field(i))
			case reflect.Int8:
				err = decodeInt8(reqValue, val.Field(i))
			case reflect.Int16:
				err = decodeInt16(reqValue, val.Field(i))
			case reflect.Int32:
				err = decodeInt32(reqValue, val.Field(i))
			case reflect.Int64:
				err = decodeInt64(reqValue, val.Field(i))
			}
			if err != nil {
				return GetErrorMessage(reqName, errMessage)
			}
		}

	}

	return nil
}

// GetErrorMessage returns the error message for the given property name
func GetErrorMessage(propertyName, customErrMesg string) error {
	if len(customErrMesg) > 0 {
		return errors.New(customErrMesg)
	}
	return errors.New("invalid " + propertyName)

}

// parseAddress parses the given input to a string
func parseAddress(input string, value reflect.Value) (err error) {
	id, err := hex.DecodeString(input)
	if err != nil {
		return
	}
	value.SetBytes(id)
	return nil
}

func parseBatchId(input string, value reflect.Value) (err error) {
	if len(input) != 64 {
		return errors.New("invalid")
	}
	err = parseAddress(input, value)
	return
}

func parseHexToHash(input string, value reflect.Value) (err error) {
	txHash := common.HexToHash(input)
	if txHash == (common.Hash{}) {
		return errors.New("invalid")
	}
	value.SetBytes(txHash.Bytes())
	return
}

func decodeUint32(input string, value reflect.Value) (err error) {
	uInt, err := strconv.ParseUint(input, 10, 32)
	if err != nil {
		return
	}

	value.SetUint(uInt)
	return nil
}

func decodeUint8(input string, value reflect.Value) (err error) {
	uInt, err := strconv.ParseUint(input, 10, 8)
	if err != nil {
		return errors.New("invalid")
	}
	value.SetUint(uInt)
	return nil
}

func decodeUint16(input string, value reflect.Value) (err error) {
	uInt, err := strconv.ParseUint(input, 10, 16)
	if err != nil {
		return errors.New("invalid")
	}
	value.SetUint(uInt)
	return nil
}

func decodeUint64(input string, value reflect.Value) (err error) {
	uInt, err := strconv.ParseUint(input, 10, 64)
	if err != nil {
		return errors.New("invalid")
	}
	value.SetUint(uInt)
	return nil
}

func decodeInt8(input string, value reflect.Value) (err error) {
	int64Value, err := strconv.ParseInt(input, 10, 8)
	if err != nil {
		return errors.New("invalid")
	}
	value.SetInt(int64Value)
	return nil
}

func decodeInt16(input string, value reflect.Value) (err error) {
	int64Value, err := strconv.ParseInt(input, 10, 16)
	if err != nil {
		return errors.New("invalid")
	}
	value.SetInt(int64Value)
	return nil
}

func decodeInt32(input string, value reflect.Value) (err error) {
	int64Value, err := strconv.ParseInt(input, 10, 32)
	if err != nil {
		return errors.New("invalid")
	}
	value.SetInt(int64Value)
	return nil
}

func decodeInt64(input string, value reflect.Value) (err error) {
	int64Value, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return errors.New("invalid")
	}
	value.SetInt(int64Value)
	return nil
}
