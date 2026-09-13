package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type BlobStore interface {
	Put(context.Context, string, []byte) error
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

type FileStore struct {
	root string
}

func NewFileStore(root string) (*FileStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("media: filesystem root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("media: resolve filesystem root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("media: create filesystem root: %w", err)
	}
	return &FileStore{root: absolute}, nil
}

func (s *FileStore) Put(_ context.Context, key string, data []byte) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("media: create blob directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".vutame-media-*")
	if err != nil {
		return fmt.Errorf("media: create temporary blob: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o640); err != nil {
		temp.Close()
		return fmt.Errorf("media: set blob permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("media: write blob: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("media: close blob: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("media: publish blob: %w", err)
	}
	return nil
}

func (s *FileStore) Open(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.pathForKey(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("media: open blob: %w", err)
	}
	return file, nil
}

func (s *FileStore) Delete(_ context.Context, key string) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("media: delete blob: %w", err)
	}
	return nil
}

func (s *FileStore) pathForKey(key string) (string, error) {
	key = filepath.Clean(strings.TrimSpace(key))
	if key == "." || key == "" || filepath.IsAbs(key) || key == ".." || strings.HasPrefix(key, ".."+string(filepath.Separator)) {
		return "", errors.New("media: invalid storage key")
	}
	path := filepath.Join(s.root, key)
	relative, err := filepath.Rel(s.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("media: storage key escapes root")
	}
	return path, nil
}
