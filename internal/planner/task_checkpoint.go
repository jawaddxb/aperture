// Package planner provides Planner implementations for Aperture.
// This file provides checkpoint save/restore for StatefulTaskPlanner.
package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// SaveCheckpoint persists a TaskContext as JSON to dir/<checkpointID>.json.
// The directory is created if it does not exist.
func SaveCheckpoint(dir string, taskCtx *domain.TaskContext) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("checkpoint: mkdir %s: %w", dir, err)
	}
	data, err := json.Marshal(taskCtx)
	if err != nil {
		return fmt.Errorf("checkpoint: marshal: %w", err)
	}
	path := filepath.Join(dir, taskCtx.CheckpointID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("checkpoint: write %s: %w", path, err)
	}
	return nil
}

// LoadCheckpoint reads and unmarshals a TaskContext from dir/<checkpointID>.json.
func LoadCheckpoint(dir, checkpointID string) (*domain.TaskContext, error) {
	path := filepath.Join(dir, checkpointID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: read %s: %w", path, err)
	}
	var taskCtx domain.TaskContext
	if err := json.Unmarshal(data, &taskCtx); err != nil {
		return nil, fmt.Errorf("checkpoint: unmarshal: %w", err)
	}
	return &taskCtx, nil
}
