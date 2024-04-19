package dynamicaccess

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

type History interface {
	Add(ctx context.Context, ref swarm.Address, timestamp *int64, metadata *map[string]string) error
	Lookup(ctx context.Context, timestamp int64) (manifest.Entry, error)
	Store(ctx context.Context) (swarm.Address, error)
}

var _ History = (*history)(nil)

var ErrEndIteration = errors.New("end iteration")

type history struct {
	manifest *manifest.MantarayManifest
	ls       file.LoadSaver
}

func NewHistory(ls file.LoadSaver) (History, error) {
	m, err := manifest.NewDefaultManifest(ls, false)
	if err != nil {
		return nil, err
	}

	mm, ok := m.(*manifest.MantarayManifest)
	if !ok {
		return nil, fmt.Errorf("expected MantarayManifest, got %T", m)
	}

	return &history{manifest: mm, ls: ls}, nil
}

func NewHistoryReference(ls file.LoadSaver, ref swarm.Address) (History, error) {
	m, err := manifest.NewDefaultManifestReference(ref, ls)
	if err != nil {
		return nil, err
	}

	mm, ok := m.(*manifest.MantarayManifest)
	if !ok {
		return nil, fmt.Errorf("expected MantarayManifest, got %T", m)
	}

	return &history{manifest: mm, ls: ls}, nil
}

func (h *history) Add(ctx context.Context, ref swarm.Address, timestamp *int64, metadata *map[string]string) error {
	mtdt := map[string]string{}
	if metadata != nil {
		mtdt = *metadata
	}
	// add timestamps transformed so that the latests timestamp becomes the smallest key
	var unixTime int64
	if timestamp != nil {
		unixTime = *timestamp
	} else {
		unixTime = time.Now().Unix()
	}

	key := strconv.FormatInt(math.MaxInt64-unixTime, 10)
	return h.manifest.Add(ctx, key, manifest.NewEntry(ref, mtdt))
}

// Lookup finds the entry for a path or returns error if not found
func (h *history) Lookup(ctx context.Context, timestamp int64) (manifest.Entry, error) {
	if timestamp <= 0 {
		return manifest.NewEntry(swarm.ZeroAddress, map[string]string{}), errors.New("invalid timestamp")
	}

	reversedTimestamp := math.MaxInt64 - timestamp
	node, err := h.lookupNode(ctx, reversedTimestamp)
	if err != nil {
		return manifest.NewEntry(swarm.ZeroAddress, map[string]string{}), err
	}

	if node != nil {
		return manifest.NewEntry(swarm.NewAddress(node.Entry()), node.Metadata()), nil
	}

	return manifest.NewEntry(swarm.ZeroAddress, map[string]string{}), nil
}

func (h *history) lookupNode(ctx context.Context, searchedTimestamp int64) (*mantaray.Node, error) {
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

func (h *history) Store(ctx context.Context) (swarm.Address, error) {
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
