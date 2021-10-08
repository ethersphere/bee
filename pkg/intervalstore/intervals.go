// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package intervalstore provides a persistence layer
// for intervals relating to a peer. The store
// provides basic operation such as adding intervals to
// existing ones and persisting the results, as well as
// getting next interval for a peer.
package intervalstore

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"sync"
)

// Intervals store a list of intervals. Its purpose is to provide
// methods to add new intervals and retrieve missing intervals that
// need to be added.
// It may be used in synchronization of streaming data to persist
// retrieved data ranges between sessions.
type Intervals struct {
	start  uint64
	ranges [][2]uint64
	mu     sync.RWMutex
}

// New creates a new instance of Intervals.
// Start argument limits the lower bound of intervals.
// No range below start bound will be added by Add method or
// returned by Next method. This limit may be used for
// tracking "live" synchronization, where the sync session
// starts from a specific value, and if "live" sync intervals
// need to be merged with historical ones, it can be safely done.
func NewIntervals(start uint64) *Intervals {
	return &Intervals{
		start: start,
	}
}

// Add adds a new range to intervals. Range start and end are values
// are both inclusive.
func (i *Intervals) Add(start, end uint64) {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.add(start, end)
}

func (i *Intervals) add(start, end uint64) {
	if start < i.start {
		start = i.start
	}
	if end < i.start {
		return
	}
	endCheck := end + 1
	if end == math.MaxUint64 {
		endCheck = end
	}
	minStartJ := -1
	maxEndJ := -1
	j := 0
	for ; j < len(i.ranges); j++ {
		if minStartJ < 0 {
			if (start <= i.ranges[j][0] && endCheck >= i.ranges[j][0]) || (start <= i.ranges[j][1]+1 && endCheck >= i.ranges[j][1]) {
				if i.ranges[j][0] < start {
					start = i.ranges[j][0]
				}
				minStartJ = j
			}
		}
		if (start <= i.ranges[j][1] && endCheck >= i.ranges[j][1]) || (start <= i.ranges[j][0] && endCheck >= i.ranges[j][0]) {
			if i.ranges[j][1] > end {
				end = i.ranges[j][1]
			}
			maxEndJ = j
		}
		if end+1 <= i.ranges[j][0] {
			break
		}
	}
	if minStartJ < 0 && maxEndJ < 0 {
		i.ranges = append(i.ranges[:j], append([][2]uint64{{start, end}}, i.ranges[j:]...)...)
		return
	}
	if minStartJ >= 0 {
		i.ranges[minStartJ][0] = start
	}
	if maxEndJ >= 0 {
		i.ranges[maxEndJ][1] = end
	}
	if minStartJ >= 0 && maxEndJ >= 0 && minStartJ != maxEndJ {
		i.ranges[maxEndJ][0] = start
		i.ranges = append(i.ranges[:minStartJ], i.ranges[maxEndJ:]...)
	}
}

// Merge adds all the intervals from the m Interval to current one.
func (i *Intervals) Merge(m *Intervals) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	i.mu.Lock()
	defer i.mu.Unlock()

	for _, r := range m.ranges {
		i.add(r[0], r[1])
	}
}

// Next returns the first range interval that is not fulfilled. Returned
// start and end values are both inclusive, meaning that the whole range
// including start and end need to be added in order to fill the gap
// in intervals.
// Returned value for end is 0 if the next interval is after the whole
// range that is stored in Intervals. Zero end value represents no limit
// on the next interval length.
// Argument ceiling is the upper bound for the returned range.
// Returned empty boolean indicates if both start and end values have
// reached the ceiling value which means that the returned range is empty,
// not containing a single element.
func (i *Intervals) Next(ceiling uint64) (start, end uint64, empty bool) {
	i.mu.RLock()
	defer func() {
		if ceiling > 0 {
			var ceilingHitStart, ceilingHitEnd bool
			if start > ceiling {
				start = ceiling
				ceilingHitStart = true
			}
			if end == 0 || end > ceiling {
				end = ceiling
				ceilingHitEnd = true
			}
			empty = ceilingHitStart && ceilingHitEnd
		}
		i.mu.RUnlock()
	}()

	l := len(i.ranges)
	if l == 0 {
		return i.start, 0, false
	}
	if i.ranges[0][0] != i.start {
		return i.start, i.ranges[0][0] - 1, false
	}
	if l == 1 {
		return i.ranges[0][1] + 1, 0, false
	}
	return i.ranges[0][1] + 1, i.ranges[1][0] - 1, false
}

// Last returns the value that is at the end of the last interval.
func (i *Intervals) Last() (end uint64) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	l := len(i.ranges)
	if l == 0 {
		return 0
	}
	return i.ranges[l-1][1]
}

// String returns a descriptive representation of range intervals
// in [] notation, as a list of two element vectors.
func (i *Intervals) String() string {
	return fmt.Sprint(i.ranges)
}

// MarshalBinary encodes Intervals parameters into a semicolon separated list.
// The first element in the list is base36-encoded start value. The following
// elements are two base36-encoded value ranges separated by comma.
func (i *Intervals) MarshalBinary() (data []byte, err error) {
	d := make([][]byte, len(i.ranges)+1)
	d[0] = []byte(strconv.FormatUint(i.start, 36))
	for j := range i.ranges {
		r := i.ranges[j]
		d[j+1] = []byte(strconv.FormatUint(r[0], 36) + "," + strconv.FormatUint(r[1], 36))
	}
	return bytes.Join(d, []byte(";")), nil
}

// UnmarshalBinary decodes data according to the Intervals.MarshalBinary format.
func (i *Intervals) UnmarshalBinary(data []byte) (err error) {
	d := bytes.Split(data, []byte(";"))
	l := len(d)
	if l == 0 {
		return nil
	}
	if l >= 1 {
		i.start, err = strconv.ParseUint(string(d[0]), 36, 64)
		if err != nil {
			return err
		}
	}
	if l == 1 {
		return nil
	}

	i.ranges = make([][2]uint64, 0, l-1)
	for j := 1; j < l; j++ {
		r := bytes.SplitN(d[j], []byte(","), 2)
		if len(r) < 2 {
			return fmt.Errorf("range %d has less then 2 elements", j)
		}
		start, err := strconv.ParseUint(string(r[0]), 36, 64)
		if err != nil {
			return fmt.Errorf("parsing the first element in range %d: %w", j, err)
		}
		end, err := strconv.ParseUint(string(r[1]), 36, 64)
		if err != nil {
			return fmt.Errorf("parsing the second element in range %d: %w", j, err)
		}
		i.add(start, end)
	}

	return nil
}
