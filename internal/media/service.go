package media

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ErrNotFound        = errors.New("media asset not found")
	ErrInvalidImage    = errors.New("invalid image")
	ErrTooLarge        = errors.New("image is too large")
	ErrProfileNotFound = errors.New("profile not found")
	ErrSchemaNotReady  = errors.New("media schema is not ready")
)

const MaxImageBytes int64 = 5 << 20

var imageExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

type Asset struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	storageKey  string
}

type OpenedAsset struct {
	Asset
	Reader io.ReadCloser
}

type Service struct {
	db    *sql.DB
	blobs BlobStore
}

func NewService(db *sql.DB, blobs BlobStore) (*Service, error) {
	if db == nil {
		return nil, errors.New("media: database is required")
	}
	if blobs == nil {
		return nil, errors.New("media: blob store is required")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'media_assets'`).Scan(&count); err != nil {
		return nil, fmt.Errorf("media: verify schema: %w", err)
	}
	if count != 1 {
		return nil, fmt.Errorf("%w: missing table media_assets; run Atlas schema apply", ErrSchemaNotReady)
	}
	return &Service{db: db, blobs: blobs}, nil
}

func (s *Service) UploadAvatar(ctx context.Context, userID string, source io.Reader) (Asset, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Asset{}, ErrProfileNotFound
	}
	data, contentType, extension, err := readImage(source)
	if err != nil {
		return Asset{}, err
	}
	id, err := randomID("med_", 18)
	if err != nil {
		return Asset{}, fmt.Errorf("media: generate asset id: %w", err)
	}
	storageKey := "avatars/" + id + extension
	if err := s.blobs.Put(ctx, storageKey, data); err != nil {
		return Asset{}, err
	}
	cleanupNew := true
	defer func() {
		if cleanupNew {
			_ = s.blobs.Delete(context.Background(), storageKey)
		}
	}()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Asset{}, fmt.Errorf("media: begin avatar replacement: %w", err)
	}
	defer tx.Rollback()

	var oldID, oldKey string
	oldErr := tx.QueryRowContext(ctx, `SELECT id, storage_key FROM media_assets WHERE user_id = ? AND slot = 'avatar'`, userID).Scan(&oldID, &oldKey)
	if oldErr != nil && !errors.Is(oldErr, sql.ErrNoRows) {
		return Asset{}, fmt.Errorf("media: load previous avatar: %w", oldErr)
	}
	if oldErr == nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_assets WHERE id = ?`, oldID); err != nil {
			return Asset{}, fmt.Errorf("media: remove previous avatar metadata: %w", err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO media_assets (id, user_id, slot, content_type, size_bytes, storage_key, created_at, updated_at)
		VALUES (?, ?, 'avatar', ?, ?, ?, ?, ?)
	`, id, userID, contentType, len(data), storageKey, now, now); err != nil {
		return Asset{}, fmt.Errorf("media: insert avatar metadata: %w", err)
	}
	url := publicURL(id)
	result, err := tx.ExecContext(ctx, `UPDATE profiles SET avatar_url = ?, updated_at = ? WHERE user_id = ?`, url, now, userID)
	if err != nil {
		return Asset{}, fmt.Errorf("media: update profile avatar: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Asset{}, fmt.Errorf("media: read profile avatar update count: %w", err)
	}
	if rows != 1 {
		return Asset{}, ErrProfileNotFound
	}
	if err := tx.Commit(); err != nil {
		return Asset{}, fmt.Errorf("media: commit avatar replacement: %w", err)
	}
	cleanupNew = false
	if oldErr == nil && oldKey != "" {
		_ = s.blobs.Delete(context.Background(), oldKey)
	}
	return Asset{ID: id, URL: url, ContentType: contentType, SizeBytes: int64(len(data)), storageKey: storageKey}, nil
}

func (s *Service) DeleteAvatar(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("media: begin avatar delete: %w", err)
	}
	defer tx.Rollback()

	var oldID, oldKey string
	oldErr := tx.QueryRowContext(ctx, `SELECT id, storage_key FROM media_assets WHERE user_id = ? AND slot = 'avatar'`, userID).Scan(&oldID, &oldKey)
	if oldErr != nil && !errors.Is(oldErr, sql.ErrNoRows) {
		return fmt.Errorf("media: load avatar metadata: %w", oldErr)
	}
	if oldErr == nil {
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_assets WHERE id = ?`, oldID); err != nil {
			return fmt.Errorf("media: delete avatar metadata: %w", err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE profiles SET avatar_url = '', updated_at = ? WHERE user_id = ?`, now, userID)
	if err != nil {
		return fmt.Errorf("media: clear profile avatar: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("media: read profile avatar clear count: %w", err)
	}
	if rows != 1 {
		return ErrProfileNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("media: commit avatar delete: %w", err)
	}
	if oldErr == nil && oldKey != "" {
		_ = s.blobs.Delete(context.Background(), oldKey)
	}
	return nil
}

func (s *Service) Open(ctx context.Context, id string) (OpenedAsset, error) {
	id = strings.TrimSpace(id)
	var contentType, storageKey string
	var sizeBytes int64
	err := s.db.QueryRowContext(ctx, `SELECT content_type, size_bytes, storage_key FROM media_assets WHERE id = ?`, id).Scan(&contentType, &sizeBytes, &storageKey)
	if errors.Is(err, sql.ErrNoRows) {
		return OpenedAsset{}, ErrNotFound
	}
	if err != nil {
		return OpenedAsset{}, fmt.Errorf("media: load asset: %w", err)
	}
	reader, err := s.blobs.Open(ctx, storageKey)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return OpenedAsset{}, ErrNotFound
		}
		return OpenedAsset{}, err
	}
	return OpenedAsset{
		Asset:  Asset{ID: id, URL: publicURL(id), ContentType: contentType, SizeBytes: sizeBytes, storageKey: storageKey},
		Reader: reader,
	}, nil
}

func publicURL(id string) string {
	return "/media/" + id
}

func readImage(source io.Reader) ([]byte, string, string, error) {
	if source == nil {
		return nil, "", "", ErrInvalidImage
	}
	limited := io.LimitReader(source, MaxImageBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", "", fmt.Errorf("media: read image: %w", err)
	}
	if int64(len(data)) > MaxImageBytes {
		return nil, "", "", ErrTooLarge
	}
	if len(data) == 0 {
		return nil, "", "", ErrInvalidImage
	}
	sniff := data
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	contentType := http.DetectContentType(sniff)
	extension, ok := imageExtensions[contentType]
	if !ok {
		return nil, "", "", fmt.Errorf("%w: supported types are JPEG, PNG, WebP, and GIF", ErrInvalidImage)
	}
	return bytes.Clone(data), contentType, extension, nil
}

func randomID(prefix string, size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buffer), nil
}
