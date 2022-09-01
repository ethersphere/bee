// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"math/big"
	"net/http"
	"reflect"
	"strconv"
)

type ValidateFunc map[string]func(interface{}, string, reflect.Value) error

var parseHooks ValidateFunc

type validateFunc func(r *http.Request, output interface{}) error

// parseAndValidateErrorResponse represents the API error response
// returned after a failed call to the parseAndValidate method.
type parseAndValidateErrorResponse struct {
	Errors string `json:"errors"`
}

func (s *Service) InitializeHooks() map[string]func(interface{}, string, reflect.Value) error {
	parseHooks = make(ValidateFunc)
	parseHooks["hexToString"] = s.parseBatchId
	return parseHooks
}

// parseAndValidate parses the input and validates it
// against the annotations declared in the given struct.
func (s *Service) parseAndValidate(input *http.Request, output interface{}, validate ...validateFunc) error {
	val := reflect.Indirect(reflect.ValueOf(output))
	reqMapVars := mux.Vars(input)
	reqMapHeaders := input.Header
	for i := 0; i < val.NumField(); i++ {
		tag := val.Type().Field(i).Tag.Get("parse")
		fmt.Println("--tag", tag)
		var reqValue string
		if varValue, isExist := reqMapVars[tag]; isExist {
			reqValue = varValue
		}
		if headerValue := reqMapHeaders.Get(tag); len(headerValue) > 0 {
			reqValue = headerValue
		}
		hook, isExist := val.Type().Field(i).Tag.Lookup("customHook")
		if isExist {
			err := parseHooks[hook](reqMapVars[tag], tag, val.Field(i))
			if err != nil {
				return errors.New("invalid " + tag)
			}
		} else {
			switch val.Type().Field(i).Type.String() {
			case "int64":
				int64Value, err := strconv.ParseInt(reqValue, 10, 64)
				if err != nil {
					return errors.New("invalid " + tag)
				}

				val.Field(i).SetInt(int64Value)
			case "uint8":
				uInt, err := strconv.ParseUint(reqValue, 10, 8)
				if err != nil {
					s.logger.Debug("create batch: parse depth string failed", "string", reqValue, "error", err)
					s.logger.Error(nil, "create batch: parse depth string failed")
					return errors.New("invalid " + tag)
				}
				val.Field(i).SetUint(uInt)
			}
		}

	}

	return nil
}

func (s *Service) ValidateDepth(r *http.Request, output interface{}) error {
	depthStr := mux.Vars(r)["depth"]
	uIntDepth, err := strconv.ParseUint(mux.Vars(r)["depth"], 10, 8)
	if err != nil {
		s.logger.Debug("create batch: parse depth string failed", "string", depthStr, "error", err)
		s.logger.Error(nil, "create batch: parse depth string failed")
		return errors.New("invalid depth")
	}
	reflect.ValueOf(output).Elem().FieldByName("Depth").SetUint(uIntDepth)

	return nil
}
func (s *Service) ValidateAmount(r *http.Request, output interface{}) error {
	_, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.logger.Error(nil, "create batch: invalid amount")
		return errors.New("invalid postage amount")
	}
	n, err := strconv.ParseInt(mux.Vars(r)["amount"], 10, 64)
	if err != nil {
		return errors.New("invalid postage amount")
	}

	reflect.ValueOf(output).Elem().FieldByName("Amount").SetInt(n)
	return nil
}

func (s *Service) parseBatchId(input interface{}, tag string, value reflect.Value) (err error) {
	if len(input.(string)) != 64 {
		s.logger.Error(nil, "get stamp issuer: invalid batch Id string length", "string", input, "length", len(input.(string)))
		return errors.New("invalid " + tag)
	}
	id, err := hex.DecodeString(input.(string))
	if err != nil {
		s.logger.Debug("get stamp issuer: decode batch Id string failed", "string", input, "error", err)
		s.logger.Error(nil, "get stamp issuer: decode batch Id string failed")
		return
	}
	value.SetBytes(id)
	//reflect.ValueOf(output).Elem().FieldByName("Id").SetString(mux.Vars(r)["Id"])
	return nil
}
