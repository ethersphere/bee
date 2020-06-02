// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package entry

import (
	"encoding/json"
)

// Metadata provides mime type and filename to file entry.
type Metadata struct {
	MimeType string `json:"mimetype"`
	Filename string `json:"filename"`
}

// NewMetadata creates a new Metadata.
func NewMetadata(fileName string) *Metadata {
	return &Metadata{
		Filename: fileName,
	}
}

func (m *Metadata) String() string {
	j, _ := json.Marshal(m)
	return string(j)
}
