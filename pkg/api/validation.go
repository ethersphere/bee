// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"github.com/gorilla/mux"
	"math/big"
	"net/http"
	"reflect"
	"strconv"
)

type validateFunc func(r *http.Request, output interface{}) error

// parseAndValidateErrorResponse represents the API error response
// returned after a failed call to the parseAndValidate method.
type parseAndValidateErrorResponse struct {
	Errors string `json:"errors"`
}

// parseAndValidate parses the input and validates it
// against the annotations declared in the given struct.
func (s *Service) parseAndValidate(input *http.Request, output interface{}, validate ...validateFunc) error {
	for _, v := range validate {
		if err := v(input, output); err != nil {
			return err
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

func (s *Service) ValidateBatchId(r *http.Request, output interface{}) error {
	if len(mux.Vars(r)["id"]) != 64 {
		s.logger.Error(nil, "get stamp issuer: invalid batch id string length", "string", mux.Vars(r)["id"], "length", len(mux.Vars(r)["id"]))
		return errors.New("invalid batchID")
	}
	reflect.ValueOf(output).Elem().FieldByName("id").SetString(mux.Vars(r)["id"])
	return nil
}
