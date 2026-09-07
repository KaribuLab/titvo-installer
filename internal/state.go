package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
)

const installStateFileName = "install-state.json"

// moduleState tracks which lifecycle steps have already completed for a module.
// Each field uses omitempty so the persisted file only shows the steps that are
// relevant for a given module (e.g. ECR-only phases or node build steps).
type moduleState struct {
	Cloned     bool   `json:"cloned,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Built      bool   `json:"built,omitempty"`
	AppliedECR bool   `json:"applied_ecr,omitempty"`
	Applied    bool   `json:"applied,omitempty"`
	Submitted  bool   `json:"submitted,omitempty"`
	Destroyed  bool   `json:"destroyed,omitempty"`
	// Synced tracks whether a static site's build output has already been
	// uploaded to S3 and its CloudFront distribution invalidated (see
	// deployStaticSiteComponent). Only used by static-site modules.
	Synced bool `json:"synced,omitempty"`
}

// resetDownstream clears build and deploy flags so a refreshed clone is
// rebuilt and re-applied. Cloned and Commit are left intact.
func (m *moduleState) resetDownstream() {
	m.Built = false
	m.AppliedECR = false
	m.Applied = false
	m.Submitted = false
	m.Destroyed = false
	m.Synced = false
}

// installState is the persisted progress of an installation run. It lives in
// ~/.titvo/install-state.json so a failed installation can be resumed instead
// of starting from scratch.
type installState struct {
	filePath string
	Modules  map[string]*moduleState `json:"modules"`
}

// loadInstallState reads the install state file from titvoDir. A missing file is
// not an error: an empty state is returned so the installation starts fresh.
func loadInstallState(titvoDir string) (*installState, error) {
	filePath := path.Join(titvoDir, installStateFileName)
	state := &installState{filePath: filePath, Modules: map[string]*moduleState{}}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, fmt.Errorf("failed to read install state file %s: %w", filePath, err)
	}

	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("failed to parse install state file %s: %w", filePath, err)
	}
	if state.Modules == nil {
		state.Modules = map[string]*moduleState{}
	}
	state.filePath = filePath
	return state, nil
}

// module returns the tracked state for a module, creating an empty entry the
// first time it is requested.
func (s *installState) module(name string) *moduleState {
	m, ok := s.Modules[name]
	if !ok {
		m = &moduleState{}
		s.Modules[name] = m
	}
	return m
}

// save persists the current state to disk atomically.
func (s *installState) save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize install state: %w", err)
	}
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write install state file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		return fmt.Errorf("failed to persist install state file %s: %w", s.filePath, err)
	}
	return nil
}

// runOnce executes action only if the step flag is not already set. On success
// it marks the step as done and persists the state, so an interrupted run can
// skip it next time.
func runOnce(state *installState, done *bool, skipMessage string, action func() error) error {
	if *done {
		printInfo(skipMessage)
		return nil
	}
	if err := action(); err != nil {
		return err
	}
	*done = true
	return state.save()
}
