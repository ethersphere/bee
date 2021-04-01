// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
)

//nolint,errcheck
func (n *Node) String() string {
	buf := bytes.NewBuffer(nil)
	io.WriteString(buf, tableCharsMap["bottom-left"])
	io.WriteString(buf, tableCharsMap["bottom"])
	io.WriteString(buf, tableCharsMap["top-right"])
	io.WriteString(buf, "\n")
	nodeStringWithPrefix(n, "  ", buf)
	return buf.String()
}

//nolint,errcheck
func nodeStringWithPrefix(n *Node, prefix string, writer io.Writer) {
	io.WriteString(writer, prefix)
	io.WriteString(writer, tableCharsMap["left-mid"])
	io.WriteString(writer, fmt.Sprintf("r: '%x'\n", n.ref))
	io.WriteString(writer, prefix)
	io.WriteString(writer, tableCharsMap["left-mid"])
	io.WriteString(writer, fmt.Sprintf("t: '%s'", strconv.FormatInt(int64(n.nodeType), 2)))
	io.WriteString(writer, " [")
	if n.IsValueType() {
		io.WriteString(writer, " Value")
	}
	if n.IsEdgeType() {
		io.WriteString(writer, " Edge")
	}
	if n.IsWithPathSeparatorType() {
		io.WriteString(writer, " PathSeparator")
	}
	io.WriteString(writer, " ]")
	io.WriteString(writer, "\n")
	io.WriteString(writer, prefix)
	if len(n.forks) > 0 || len(n.metadata) > 0 {
		io.WriteString(writer, tableCharsMap["left-mid"])
	} else {
		io.WriteString(writer, tableCharsMap["bottom-left"])
	}
	io.WriteString(writer, fmt.Sprintf("e: '%s'\n", string(n.entry)))
	if len(n.metadata) > 0 {
		io.WriteString(writer, prefix)
		if len(n.forks) > 0 {
			io.WriteString(writer, tableCharsMap["left-mid"])
		} else {
			io.WriteString(writer, tableCharsMap["bottom-left"])
		}
		io.WriteString(writer, fmt.Sprintf("m: '%s'\n", n.metadata))
	}
	counter := 0
	for k, f := range n.forks {
		isLast := counter != len(n.forks)-1
		io.WriteString(writer, prefix)
		if isLast {
			io.WriteString(writer, tableCharsMap["left-mid"])
		} else {
			io.WriteString(writer, tableCharsMap["bottom-left"])
		}
		io.WriteString(writer, tableCharsMap["mid"])
		io.WriteString(writer, fmt.Sprintf("[%s]", string(k)))
		io.WriteString(writer, tableCharsMap["mid"])
		io.WriteString(writer, tableCharsMap["top-mid"])
		io.WriteString(writer, tableCharsMap["mid"])
		io.WriteString(writer, fmt.Sprintf("`%s`\n", string(f.prefix)))
		newPrefix := prefix
		if isLast {
			newPrefix += tableCharsMap["middle"]
		} else {
			newPrefix += " "
		}
		newPrefix += "     "
		nodeStringWithPrefix(f.Node, newPrefix, writer)
		counter++
	}
}

var tableCharsMap = map[string]string{
	"top":          "─",
	"top-mid":      "┬",
	"top-left":     "┌",
	"top-right":    "┐",
	"bottom":       "─",
	"bottom-mid":   "┴",
	"bottom-left":  "└",
	"bottom-right": "┘",
	"left":         "│",
	"left-mid":     "├",
	"mid":          "─",
	"mid-mid":      "┼",
	"right":        "│",
	"right-mid":    "┤",
	"middle":       "│",
}
