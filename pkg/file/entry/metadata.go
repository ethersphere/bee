// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry

import (
	"bytes"
	"fmt"
)

type Metadata struct {
	MimeType string
	Filename string
}

func NewMetadata(fileName string) *Metadata {
	return &Metadata{
		Filename: fileName,
	}
}

func (m *Metadata) WithMimeType(mimeType string) *Metadata {
	m.MimeType = mimeType
	return m
}

func (m *Metadata) MarshalBinary() ([]byte, error) {
	b := []byte(m.Filename)
	if m.MimeType != "" {
		bs := append(b, []byte{0x00}...)
		b = append(bs, m.MimeType...)
	}
	return b, nil
}

func (m *Metadata) UnmarshalBinary(b []byte) error {
	metadataBytes := bytes.Split(b, []byte{0x00})
	if len(metadataBytes) != 2 {
		return fmt.Errorf("invalid metadata")
	}
	m.Filename = fmt.Sprintf("%s", metadataBytes[0])
	m.MimeType = fmt.Sprintf("%s", metadataBytes[1])
	return nil
}
