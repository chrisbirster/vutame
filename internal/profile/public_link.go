package profile

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type PublicLinkTarget struct {
	UserID string
	Link   Link
}

type PublicLinkResolver interface {
	ResolvePublicLink(string) (PublicLinkTarget, error)
}

func (s *MemoryStore) ResolvePublicLink(linkID string) (PublicLinkTarget, error) {
	linkID = strings.TrimSpace(linkID)
	if linkID == "" {
		return PublicLinkTarget{}, ErrNotFound
	}
	now := time.Now().UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.byHandle {
		for _, link := range item.Links {
			if link.ID != linkID {
				continue
			}
			if !link.IsActive || !linkVisibleAt(link, now) {
				return PublicLinkTarget{}, ErrNotFound
			}
			link.Kind = NormalizeLinkKind(link.Kind)
			return PublicLinkTarget{UserID: item.ID, Link: link}, nil
		}
	}
	return PublicLinkTarget{}, ErrNotFound
}

func (s *SQLiteStore) ResolvePublicLink(linkID string) (PublicLinkTarget, error) {
	linkID = strings.TrimSpace(linkID)
	if linkID == "" {
		return PublicLinkTarget{}, ErrNotFound
	}
	var target PublicLinkTarget
	var featured int
	var active int
	var visibleFrom sql.NullString
	var visibleUntil sql.NullString
	err := s.db.QueryRow(`
		SELECT user_id, id, label, url, kind, thumbnail_url, featured, visible_from, visible_until, position, is_active
		FROM links
		WHERE id = ?
	`, linkID).Scan(
		&target.UserID,
		&target.Link.ID,
		&target.Link.Label,
		&target.Link.URL,
		&target.Link.Kind,
		&target.Link.ThumbnailURL,
		&featured,
		&visibleFrom,
		&visibleUntil,
		&target.Link.Position,
		&active,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PublicLinkTarget{}, ErrNotFound
	}
	if err != nil {
		return PublicLinkTarget{}, fmt.Errorf("resolve public link: %w", err)
	}
	target.Link.Featured = featured != 0
	target.Link.IsActive = active != 0
	if visibleFrom.Valid {
		target.Link.VisibleFrom = visibleFrom.String
	}
	if visibleUntil.Valid {
		target.Link.VisibleUntil = visibleUntil.String
	}
	if !target.Link.IsActive || !linkVisibleAt(target.Link, time.Now().UTC()) {
		return PublicLinkTarget{}, ErrNotFound
	}
	target.Link.Kind = NormalizeLinkKind(target.Link.Kind)
	return target, nil
}
