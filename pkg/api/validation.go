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

type ValidateFunc map[string]func(interface{}, string, reflect.Value) error

var parseHooks ValidateFunc

type validateFunc func(r *http.Request, output interface{}) error

// parseAndValidateErrorResponse represents the API error response
// returned after a failed call to the parseAndValidate method.
//type parseAndValidateErrorResponse struct {
//	Errors string `json:"errors"`
//}

func (s *Service) InitializeHooks() map[string]func(interface{}, string, reflect.Value) error {
	parseHooks = make(ValidateFunc)
	parseHooks["hexToString"] = s.parseBatchId
	return parseHooks
}

// parseAndValidate parses the input and validates it
// against the annotations declared in the given struct.
func (s *Service) parseAndValidate(input *http.Request, output interface{}, validate ...validateFunc) error {
	val := reflect.Indirect(reflect.ValueOf(output))
	//decoder := mapstructure.Decoder{}
	reqMapVars := mux.Vars(input)
	reqMapHeaders := input.Header

	if input.Body != nil {
		body, err := io.ReadAll(input.Body)
		if err != nil {
			s.logger.Debug("done split tag: read request body failed", "error", err)
			s.logger.Error(nil, "done split tag: read request body failed")
			return errors.New("cannot read request")
		}
		if len(body) > 0 {
			err = json.Unmarshal(body, &output)
			if err != nil {
				s.logger.Debug("done split tag: unmarshal tag name failed", "error", err)
				s.logger.Error(nil, "done split tag: unmarshal tag name failed")
				return errors.New("cannot read request")
			}

		}
	}
	for i := 0; i < val.NumField(); i++ {
		tag := val.Type().Field(i).Tag.Get("parse")
		propertyName := val.Type().Field(i).Tag.Get("name")
		errMessage := val.Type().Field(i).Tag.Get("errMessage")
		fmt.Println("--tag", tag)
		var reqValue string
		if varValue, isExist := reqMapVars[tag]; isExist {
			reqValue = varValue
		}
		if headerValue := reqMapHeaders.Get(tag); len(headerValue) > 0 {
			reqValue = headerValue
		}
		hook, isExist := val.Type().Field(i).Tag.Lookup("customHook")
		fmt.Println("--hook", hook)
		if isExist {
			err := parseHooks[hook](reqMapVars[tag], tag, val.Field(i))
			if err != nil {
				return s.GetErrorMessage(propertyName, errMessage)
			}
		} else {
			switch val.Type().Field(i).Type.Kind() {
			case reflect.Uint32:
				if err := s.decodeUint(reqValue, val.Field(i)); err != nil {
					return s.GetErrorMessage(propertyName, errMessage)
				}

			case reflect.Int64:
				int64Value, err := strconv.ParseInt(reqValue, 10, 64)
				if err != nil {
					return s.GetErrorMessage(propertyName, errMessage)
				}

				val.Field(i).SetInt(int64Value)
			case reflect.Uint8:
				uInt, err := strconv.ParseUint(reqValue, 10, 8)
				if err != nil {
					s.logger.Debug("create batch: parse depth string failed", "string", reqValue, "error", err)
					s.logger.Error(nil, "create batch: parse depth string failed")
					return s.GetErrorMessage(propertyName, errMessage)
				}
				val.Field(i).SetUint(uInt)
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
func (s *Service) parseBatchId(input interface{}, tag string, value reflect.Value) (err error) {
	if len(input.(string)) != 64 {
		s.logger.Error(nil, "invalid batch Id string length", "string", input, "length", len(input.(string)))
		return errors.New("invalid " + tag)
	}
	id, err := hex.DecodeString(input.(string))
	if err != nil {
		s.logger.Debug("decode batch Id string failed", "string", input, "error", err)
		s.logger.Error(nil, "decode batch Id string failed")
		return
	}
	value.SetBytes(id)
	return nil
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
