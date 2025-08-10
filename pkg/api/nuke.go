// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
)

// NukeService handles the long-running nuke operation
type NukeService struct {
	logger    log.Logger
	mu        sync.RWMutex
	status    NukeStatus
	startTime time.Time
	endTime   *time.Time
	error     error
}

// NukeStatus represents the current status of the nuke operation
type NukeStatus string

const (
	NukeStatusNotStarted NukeStatus = "not_started"
	NukeStatusInProgress NukeStatus = "in_progress"
	NukeStatusCompleted  NukeStatus = "completed"
	NukeStatusFailed     NukeStatus = "failed"
)

// NukeStatusResponse represents the response for the status endpoint
type NukeStatusResponse struct {
	Status    NukeStatus `json:"status"`
	StartTime time.Time  `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// NukeRequest represents the request to start a nuke operation
type NukeRequest struct {
	DataDir       string `json:"data_dir" validate:"required"`
	ForgetOverlay bool   `json:"forget_overlay"`
	ForgetStamps  bool   `json:"forget_stamps"`
}

// NewNukeService creates a new nuke service
func NewNukeService(logger log.Logger) *NukeService {
	return &NukeService{
		logger: logger,
		status: NukeStatusNotStarted,
	}
}

// StartNuke starts the nuke operation in a goroutine
func (s *NukeService) StartNuke(req NukeRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == NukeStatusInProgress {
		return fmt.Errorf("nuke operation already in progress")
	}

	s.status = NukeStatusInProgress
	s.startTime = time.Now()
	s.endTime = nil
	s.error = nil

	go s.executeNuke(req)

	return nil
}

// executeNuke performs the actual nuke operation
func (s *NukeService) executeNuke(req NukeRequest) {
	defer func() {
		s.mu.Lock()
		if s.status == NukeStatusInProgress {
			if s.error != nil {
				s.status = NukeStatusFailed
			} else {
				s.status = NukeStatusCompleted
			}
		}
		now := time.Now()
		s.endTime = &now
		s.mu.Unlock()
	}()

	s.logger.Warning("starting to nuke the DB with data-dir", "path", req.DataDir)
	s.logger.Warning("this process will erase all persisted chunks in your local storage")
	s.logger.Warning("it will NOT discriminate any pinned content, in case you were wondering")

	// Give user time to cancel
	s.logger.Warning("you have another 10 seconds to change your mind and kill this process...")
	time.Sleep(10 * time.Second)
	s.logger.Warning("proceeding with database nuke...")

	const (
		localstore   = "localstore"
		kademlia     = "kademlia"
		statestore   = "statestore"
		stamperstore = "stamperstore"
	)

	dirsToNuke := []string{localstore, kademlia}
	for _, dir := range dirsToNuke {
		err := s.removeContent(filepath.Join(req.DataDir, dir))
		if err != nil {
			s.error = fmt.Errorf("delete %s: %w", dir, err)
			return
		}
	}

	if req.ForgetOverlay {
		err := s.removeContent(filepath.Join(req.DataDir, statestore))
		if err != nil {
			s.error = fmt.Errorf("remove statestore: %w", err)
			return
		}
		err = s.removeContent(filepath.Join(req.DataDir, stamperstore))
		if err != nil {
			s.error = fmt.Errorf("remove stamperstore: %w", err)
			return
		}
		return
	}

	s.logger.Info("nuking statestore...")

	// For now, we'll just remove the statestore directory content
	// The actual statestore nuking will be done when the node restarts
	if req.ForgetStamps {
		err := s.removeContent(filepath.Join(req.DataDir, stamperstore))
		if err != nil {
			s.error = fmt.Errorf("remove stamperstore: %w", err)
			return
		}
	}

	s.logger.Info("nuke operation completed successfully")
}

// removeContent removes all content from a directory
func (s *NukeService) removeContent(path string) error {
	dir, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()

	subpaths, err := dir.Readdirnames(0)
	if err != nil {
		return err
	}

	for _, sub := range subpaths {
		err = os.RemoveAll(filepath.Join(path, sub))
		if err != nil {
			return err
		}
	}
	return nil
}

// GetStatus returns the current status of the nuke operation
func (s *NukeService) GetStatus() NukeStatusResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	response := NukeStatusResponse{
		Status:    s.status,
		StartTime: s.startTime,
		EndTime:   s.endTime,
	}

	if s.error != nil {
		response.Error = s.error.Error()
	}

	return response
}

// IsReady returns true if the service is ready to accept new nuke requests
func (s *NukeService) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status != NukeStatusInProgress
}

// nukeHandler handles the POST request to start a nuke operation
func (s *Service) nukeHandler(w http.ResponseWriter, r *http.Request) {
	if s.nukeService == nil {
		jsonhttp.BadRequest(w, "nuke service not available")
		return
	}

	var req NukeRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		jsonhttp.BadRequest(w, "invalid request body")
		return
	}

	if err := s.nukeService.StartNuke(req); err != nil {
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	jsonhttp.OK(w, "nuke operation started")
}

// nukeStatusHandler handles the GET request to get nuke operation status
func (s *Service) nukeStatusHandler(w http.ResponseWriter, r *http.Request) {
	if s.nukeService == nil {
		jsonhttp.BadRequest(w, "nuke service not available")
		return
	}

	status := s.nukeService.GetStatus()
	jsonhttp.OK(w, status)
}

// nukeReadinessHandler handles the readiness probe for the nuke service
func (s *Service) nukeReadinessHandler(w http.ResponseWriter, r *http.Request) {
	if s.nukeService == nil {
		jsonhttp.BadRequest(w, "nuke service not available")
		return
	}

	if s.nukeService.IsReady() {
		jsonhttp.OK(w, "ready")
	} else {
		jsonhttp.ServiceUnavailable(w, "nuke operation in progress")
	}
}
