// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strconv"
	"strings"
)

func parseGraphvizPlain(data []byte) (*graphvizLayout, error) {
	layout := &graphvizLayout{
		Nodes: map[string]graphvizNodeLayout{},
		Edges: map[string]graphvizEdgeLayout{},
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "stop" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "graph "):
			if err := parsePlainGraphLine(line, layout); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "node "):
			name, loc, err := parsePlainNodeLine(line)
			if err != nil {
				return nil, err
			}
			layout.Nodes[unquotePlainToken(name)] = loc
		case strings.HasPrefix(line, "edge "):
			tail, head, pts, err := parsePlainEdgeLine(line)
			if err != nil {
				return nil, err
			}
			layout.Edges[unquotePlainToken(tail)+"\x00"+unquotePlainToken(head)] = graphvizEdgeLayout{Points: pts}
		}
	}
	if layout.HeightInches == 0 {
		return nil, fmt.Errorf("graphviz plain output missing graph size")
	}
	return layout, nil
}

func parsePlainGraphLine(line string, layout *graphvizLayout) error {
	rest := strings.TrimPrefix(line, "graph ")
	fields, err := readPlainFloats(rest, 3)
	if err != nil {
		return fmt.Errorf("graph line: %w", err)
	}
	layout.WidthInches = fields[1]
	layout.HeightInches = fields[2]
	return nil
}

func parsePlainNodeLine(line string) (name string, loc graphvizNodeLayout, err error) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "node "))
	name, rest, ok := readPlainToken(rest)
	if !ok {
		return "", graphvizNodeLayout{}, fmt.Errorf("node line: %q", line)
	}
	fields, err := readPlainFloats(rest, 4)
	if err != nil {
		return "", graphvizNodeLayout{}, fmt.Errorf("node line: %q", line)
	}
	return name, graphvizNodeLayout{
		CenterX: fields[0], CenterY: fields[1], Width: fields[2], Height: fields[3],
	}, nil
}

func parsePlainEdgeLine(line string) (tail, head string, pts []graphvizPoint, err error) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "edge "))
	tail, rest, ok := readPlainToken(rest)
	if !ok {
		return "", "", nil, fmt.Errorf("edge line: %q", line)
	}
	head, rest, ok = readPlainToken(rest)
	if !ok {
		return "", "", nil, fmt.Errorf("edge line: %q", line)
	}
	nTok, rest, ok := readPlainToken(rest)
	if !ok {
		return "", "", nil, fmt.Errorf("edge line: %q", line)
	}
	nPts, err := strconv.Atoi(nTok)
	if err != nil {
		return "", "", nil, fmt.Errorf("edge point count: %w", err)
	}
	coords, err := readPlainFloats(rest, nPts*2)
	if err != nil {
		return "", "", nil, fmt.Errorf("edge points: %w", err)
	}
	pts = make([]graphvizPoint, 0, nPts)
	for i := 0; i < len(coords); i += 2 {
		pts = append(pts, graphvizPoint{X: coords[i], Y: coords[i+1]})
	}
	return tail, head, pts, nil
}

func readPlainToken(s string) (token, rest string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	if s[0] == '"' {
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' && i+1 < len(s) {
				b.WriteByte(s[i+1])
				i++
				continue
			}
			if s[i] == '"' {
				return b.String(), strings.TrimSpace(s[i+1:]), true
			}
			b.WriteByte(s[i])
		}
		return "", "", false
	}
	i := strings.IndexByte(s, ' ')
	if i < 0 {
		return s, "", true
	}
	return s[:i], strings.TrimSpace(s[i+1:]), true
}

func readPlainFloats(s string, count int) ([]float64, error) {
	out := make([]float64, 0, count)
	rest := s
	for len(out) < count {
		tok, next, ok := readPlainToken(rest)
		if !ok {
			return nil, fmt.Errorf("expected %d floats, got %d in %q", count, len(out), s)
		}
		v, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		rest = next
	}
	return out, nil
}

func unquotePlainToken(s string) string {
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2 {
		return s[1 : len(s)-1]
	}
	return s
}
