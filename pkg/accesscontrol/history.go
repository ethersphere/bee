// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/manifest/mantaray"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	// ErrEndIteration indicates that the iteration terminated.
	ErrEndIteration = errors.New("end iteration")
	// ErrUnexpectedType indicates that an error occurred during the mantary-manifest creation.
	ErrUnexpectedType = errors.New("unexpected type")
	// ErrInvalidTimestamp indicates that the timestamp given to Lookup is invalid.
	ErrInvalidTimestamp = errors.New("invalid timestamp")
	// ErrNotFound is returned when an Entry is not found in the history.
	ErrNotFound = errors.New("access control: not found")
)

// History represents the interface for managing access control history.
type History interface {
	// Add adds a new entry to the access control history with the given timestamp and metadata.
	Add(ctx context.Context, ref swarm.Address, timestamp *int64, metadata *map[string]string) error
	// Lookup retrieves the entry from the history based on the given timestamp or returns error if not found.
	Lookup(ctx context.Context, timestamp int64) (manifest.Entry, error)
	// Store stores the history to the underlying storage and returns the reference.
	Store(ctx context.Context) (swarm.Address, error)
}

var _ History = (*HistoryStruct)(nil)

// manifestInterface extends the `manifest.Interface` interface and adds a `Root` method.
type manifestInterface interface {
	manifest.Interface
	Root() *mantaray.Node
}

// HistoryStruct represents an access control history with a mantaray-based manifest.
type HistoryStruct struct {
	manifest manifestInterface
	ls       file.LoadSaver
}

// NewHistory creates a new history with a mantaray-based manifest.
func NewHistory(ls file.LoadSaver) (*HistoryStruct, error) {
	m, err := manifest.NewMantarayManifest(ls, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create mantaray manifest: %w", err)
	}

	mm, ok := m.(manifestInterface)
	if !ok {
		return nil, fmt.Errorf("%w: expected MantarayManifest, got %T", ErrUnexpectedType, m)
	}

	return &HistoryStruct{manifest: mm, ls: ls}, nil
}

// NewHistoryReference loads a history with a mantaray-based manifest.
func NewHistoryReference(ls file.LoadSaver, ref swarm.Address) (*HistoryStruct, error) {
	m, err := manifest.NewMantarayManifestReference(ref, ls)
	if err != nil {
		return nil, fmt.Errorf("failed to create mantaray manifest reference: %w", err)
	}

	mm, ok := m.(manifestInterface)
	if !ok {
		return nil, fmt.Errorf("%w: expected MantarayManifest, got %T", ErrUnexpectedType, m)
	}

	return &HistoryStruct{manifest: mm, ls: ls}, nil
}

// Add adds a new entry to the access control history with the given timestamp and metadata.
func (h *HistoryStruct) Add(ctx context.Context, ref swarm.Address, timestamp *int64, metadata *map[string]string) error {
	mtdt := map[string]string{}
	if metadata != nil {
		mtdt = *metadata
	}
	// add timestamps transformed so that the latests timestamp becomes the smallest key.
	unixTime := time.Now().Unix()
	if timestamp != nil {
		unixTime = *timestamp
	}

	key := strconv.FormatInt(math.MaxInt64-unixTime, 10)
	err := h.manifest.Add(ctx, key, manifest.NewEntry(ref, mtdt))
	if err != nil {
		return fmt.Errorf("failed to add to manifest: %w", err)
	}

	return nil
}

// Lookup retrieves the entry from the history based on the given timestamp or returns error if not found.
func (h *HistoryStruct) Lookup(ctx context.Context, timestamp int64) (manifest.Entry, error) {
	if timestamp <= 0 {
		return manifest.NewEntry(swarm.ZeroAddress, map[string]string{}), ErrInvalidTimestamp
	}

	reversedTimestamp := math.MaxInt64 - timestamp
	node, err := h.lookupNode(ctx, reversedTimestamp)
	if err != nil {
		switch {
		case errors.Is(err, manifest.ErrNotFound):
			return manifest.NewEntry(swarm.ZeroAddress, map[string]string{}), ErrNotFound
		default:
			return manifest.NewEntry(swarm.ZeroAddress, map[string]string{}), err
		}
	}

	if node != nil {
		return manifest.NewEntry(swarm.NewAddress(node.Entry()), node.Metadata()), nil
	}

	return manifest.NewEntry(swarm.ZeroAddress, map[string]string{}), ErrNotFound
}

func (h *HistoryStruct) lookupNode(ctx context.Context, searchedTimestamp int64) (*mantaray.Node, error) {
	// before node's timestamp is the closest one that is less than or equal to the searched timestamp
	// for instance: 2030, 2020, 1994 -> search for 2021 -> before is 2020
	var beforeNode *mantaray.Node
	// after node's timestamp is after the latest
	// for instance: 2030, 2020, 1994 -> search for 1980 -> after is 1994
	var afterNode *mantaray.Node

	walker := func(pathTimestamp []byte, currNode *mantaray.Node, err error) error {
		if err != nil {
			return err
		}

		if currNode.IsValueType() && len(currNode.Entry()) > 0 {
			afterNode = currNode

			match, err := isBeforeMatch(pathTimestamp, searchedTimestamp)
			if match {
				beforeNode = currNode
				// return error to stop the walk, this is how WalkNode works...
				return ErrEndIteration
			}

			return err
		}

		return nil
	}

	rootNode := h.manifest.Root()
	err := rootNode.WalkNode(ctx, []byte{}, h.ls, walker)

	if err != nil && !errors.Is(err, ErrEndIteration) {
		return nil, fmt.Errorf("history lookup node error: %w", err)
	}

	if beforeNode != nil {
		return beforeNode, nil
	}
	if afterNode != nil {
		return afterNode, nil
	}
	return nil, nil
}

// Store stores the history to the underlying storage and returns the reference.
func (h *HistoryStruct) Store(ctx context.Context) (swarm.Address, error) {
	return h.manifest.Store(ctx)
}

func bytesToInt64(b []byte) (int64, error) {
	num, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return -1, err
	}

	return num, nil
}

func isBeforeMatch(pathTimestamp []byte, searchedTimestamp int64) (bool, error) {
	targetTimestamp, err := bytesToInt64(pathTimestamp)
	if err != nil {
		return false, err
	}
	if targetTimestamp == 0 {
		return false, nil
	}
	return searchedTimestamp <= targetTimestamp, nil
}
