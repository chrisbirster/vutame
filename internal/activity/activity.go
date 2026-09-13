package activity

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidCursor = errors.New("invalid activity cursor")

const (
	KindProfileUpdated = "profile_updated"
	KindLinkFeatured   = "link_featured"
	DefaultLimit       = 20
	MaxLimit           = 50
)

type Event struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	LinkID      string `json:"link_id,omitempty"`
	Label       string `json:"label,omitempty"`
	CreatedAt   string `json:"created_at"`
}

type Page struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

type Trend struct {
	Handle         string `json:"handle"`
	DisplayName    string `json:"display_name"`
	AvatarURL      string `json:"avatar_url,omitempty"`
	RecentActivity int    `json:"recent_activity"`
	FollowerCount  int    `json:"follower_count"`
	Score          int    `json:"score"`
}

type Store interface {
	Record(context.Context, string, string, string, string) error
	Following(context.Context, string, string, int) (Page, error)
	Recent(context.Context, string, int) (Page, error)
	Trending(context.Context, int) ([]Trend, error)
}

func NormalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultLimit
	}
	if limit > MaxLimit {
		return MaxLimit
	}
	return limit
}

func EncodeCursor(createdAt, id string) string {
	if createdAt == "" || id == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "\x00" + id))
}

func DecodeCursor(cursor string) (string, string, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return "", "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", ErrInvalidCursor
	}
	parts := strings.SplitN(string(decoded), "\x00", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ErrInvalidCursor
	}
	return parts[0], parts[1], nil
}

func ValidateKind(kind string) error {
	if kind != KindProfileUpdated && kind != KindLinkFeatured {
		return fmt.Errorf("unsupported activity kind %q", kind)
	}
	return nil
}
