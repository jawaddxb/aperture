package planner_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ApertureHQ/aperture/internal/domain"
	"github.com/ApertureHQ/aperture/internal/planner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndLoadCheckpoint(t *testing.T) {
	dir := t.TempDir()

	raw, _ := json.Marshal(map[string]string{"name": "Alice"})
	taskCtx := &domain.TaskContext{
		ID:           "test-task-1",
		Goal:         "Extract LinkedIn connections",
		Mode:         "research",
		Status:       "executing",
		CurrentStep:  3,
		TotalSteps:   9,
		ExtractCount: 15,
		Extracted:    []json.RawMessage{raw},
		CurrentPage:  1,
		TotalPages:   3,
		HasMore:      true,
		CheckpointID: "test-task-1",
		CheckpointAt: time.Now(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err := planner.SaveCheckpoint(dir, taskCtx)
	require.NoError(t, err)

	loaded, err := planner.LoadCheckpoint(dir, "test-task-1")
	require.NoError(t, err)

	assert.Equal(t, taskCtx.ID, loaded.ID)
	assert.Equal(t, taskCtx.Goal, loaded.Goal)
	assert.Equal(t, taskCtx.Mode, loaded.Mode)
	assert.Equal(t, taskCtx.Status, loaded.Status)
	assert.Equal(t, taskCtx.CurrentStep, loaded.CurrentStep)
	assert.Equal(t, taskCtx.TotalSteps, loaded.TotalSteps)
	assert.Equal(t, taskCtx.ExtractCount, loaded.ExtractCount)
	assert.Equal(t, taskCtx.HasMore, loaded.HasMore)
	assert.Len(t, loaded.Extracted, 1)
}

func TestLoadCheckpoint_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := planner.LoadCheckpoint(dir, "does-not-exist")
	assert.Error(t, err)
}

func TestSaveCheckpoint_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	subDir := dir + "/nested/checkpoints"

	taskCtx := &domain.TaskContext{
		ID:           "nested-task",
		CheckpointID: "nested-task",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err := planner.SaveCheckpoint(subDir, taskCtx)
	require.NoError(t, err)

	_, statErr := os.Stat(subDir)
	assert.NoError(t, statErr)
}
