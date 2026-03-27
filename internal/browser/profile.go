package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ApertureHQ/aperture/internal/domain"
)

// FileProfileManager implements domain.ProfileManager by using the filesystem.
type FileProfileManager struct {
	baseDir string
}

// NewFileProfileManager creates a manager that stores profiles in baseDir.
func NewFileProfileManager(baseDir string) (*FileProfileManager, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create profiles base directory: %w", err)
	}
	return &FileProfileManager{baseDir: baseDir}, nil
}

// CreateProfile creates a new profile directory and returns its details.
func (m *FileProfileManager) CreateProfile(ctx context.Context, id string) (*domain.Profile, error) {
	path := filepath.Join(m.baseDir, id)
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create profile directory: %w", err)
	}

	return &domain.Profile{
		ID:      id,
		Path:    path,
		Context: "default",
	}, nil
}

// DeleteProfile removes the profile directory from disk.
func (m *FileProfileManager) DeleteProfile(ctx context.Context, id string) error {
	path := filepath.Join(m.baseDir, id)
	return os.RemoveAll(path)
}
