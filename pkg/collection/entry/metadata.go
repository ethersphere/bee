// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry

import (
	"bytes"
	"fmt"
)

// Metadata provides mime type and filename to file entry.
type Metadata struct {
	MimeType string
	Filename string
}

// NewMetadata creates a new Metadata.
func NewMetadata(fileName string) *Metadata {
	return &Metadata{
		Filename: fileName,
	}
}

// WithMimeType adds mime type to Metadata.
func (m *Metadata) WithMimeType(mimeType string) *Metadata {
	m.MimeType = mimeType
	return m
}

// MarshalBinary implements encoding.BinaryMarshaler
func (m *Metadata) MarshalBinary() ([]byte, error) {
	b := []byte(m.Filename)
	if m.MimeType != "" {
		bs := append(b, []byte{0x00}...)
		b = append(bs, m.MimeType...)
	}
	return b, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (m *Metadata) UnmarshalBinary(b []byte) error {
	metadataBytes := bytes.Split(b, []byte{0x00})
	if len(metadataBytes) != 2 {
		return fmt.Errorf("invalid metadata")
	}
	m.Filename = fmt.Sprintf("%s", metadataBytes[0])
	m.MimeType = fmt.Sprintf("%s", metadataBytes[1])
	return nil
}
