// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"net/http"
	"reflect"
	"strconv"
)

type ValidateFunc map[string]func(interface{}, reflect.Value) error

var parseHooks ValidateFunc

type validateFunc func(r *http.Request, output interface{}) error

func (s *Service) InitializeHooks() ValidateFunc {
	parseHooks = make(ValidateFunc)
	parseHooks["hexToString"] = s.parseBatchId
	parseHooks["addressToString"] = s.parseAddress
	return parseHooks
}

// parseAndValidate parses the input and validates it
// against the annotations declared in the given struct.
func (s *Service) parseAndValidate(input *http.Request, output interface{}) (err error) {
	val := reflect.Indirect(reflect.ValueOf(output))

	reqMapVars := mux.Vars(input)
	reqMapHeaders := input.Header
	reqMapQuery := input.URL.Query()

	for i := 0; i < val.NumField(); i++ {
		tag := val.Type().Field(i).Tag.Get("parse")
		propertyName := val.Type().Field(i).Tag.Get("name")
		errMessage := val.Type().Field(i).Tag.Get("errMessage")
		if val.Type().Field(i).Name == "RequestData" && input.Body != nil {
			body, err := io.ReadAll(input.Body)
			if err != nil {
				s.logger.Debug("done split tag: read request body failed", "error", err)
				s.logger.Error(nil, "done split tag: read request body failed")
				return s.GetErrorMessage(propertyName, errMessage)
			}
			if len(body) > 0 {
				err = json.Unmarshal(body, &output)
				if err != nil {
					s.logger.Debug("unmarshal tag name failed", "error", err)
					s.logger.Error(nil, "unmarshal tag name failed")
					return s.GetErrorMessage(propertyName, errMessage)
				}

			}
		}
		fmt.Println("--tag", tag)
		fmt.Println("--val.Type().Field(i).Name", val.Type().Field(i).Name)
		var reqValue string
		if varValue, isExist := reqMapVars[tag]; isExist {
			reqValue = varValue
		}
		if headerValue := reqMapHeaders.Get(tag); len(headerValue) > 0 {
			reqValue = headerValue
		}
		if queryValue := reqMapQuery.Get(tag); len(queryValue) > 0 {
			reqValue = queryValue
		}
		hook, isExist := val.Type().Field(i).Tag.Lookup("customHook")
		fmt.Println("--hook", hook)
		if isExist {
			err = parseHooks[hook](reqMapVars[tag], val.Field(i))
			if err != nil {
				return s.GetErrorMessage(propertyName, errMessage)
			}
		} else {
			switch val.Type().Field(i).Type.Kind() {
			case reflect.Uint32:
				err = s.decodeUint(reqValue, val.Field(i))
			case reflect.Int64:
				err = s.decodeInt64(reqValue, val.Field(i))
			case reflect.Uint8:
				err = s.decodeUint8(reqValue, val.Field(i))
			}
			if err != nil {
				return s.GetErrorMessage(propertyName, errMessage)
			}
		}

	}

	return nil
}

func (s *Service) GetErrorMessage(propertyName, customErrMesg string) error {
	if len(customErrMesg) > 0 {
		return errors.New(customErrMesg)
	}
	return errors.New("invalid " + propertyName)

}
func (s *Service) parseAddress(input interface{}, value reflect.Value) (err error) {
	id, err := hex.DecodeString(input.(string))
	if err != nil {
		s.logger.Debug("decode id string failed", "string", input, "error", err)
		s.logger.Error(nil, "decode id string failed")
		return
	}
	value.SetBytes(id)
	return nil
}
func (s *Service) parseBatchId(input interface{}, value reflect.Value) (err error) {
	if len(input.(string)) != 64 {
		s.logger.Error(nil, "invalid batch Id string length", "string", input, "length", len(input.(string)))
		return errors.New("invalid")
	}
	err = s.parseAddress(input, value)
	return
}

func (s *Service) decodeUint(input interface{}, value reflect.Value) (err error) {
	uInt, err := strconv.ParseUint(input.(string), 10, 32)
	if err != nil {
		s.logger.Debug("parse depth string failed", "string", input.(string), "error", err)
		s.logger.Error(nil, "create batch: parse depth string failed")
		return
	}
	value.SetUint(uInt)
	return nil
}

func (s *Service) decodeInt64(input interface{}, value reflect.Value) (err error) {
	int64Value, err := strconv.ParseInt(input.(string), 10, 64)
	if err != nil {
		return errors.New("invalid")
	}
	value.SetInt(int64Value)
	return nil
}

func (s *Service) decodeUint8(input interface{}, value reflect.Value) (err error) {
	uInt, err := strconv.ParseUint(input.(string), 10, 8)
	if err != nil {
		return errors.New("invalid")
	}
	value.SetUint(uInt)
	return nil
}
