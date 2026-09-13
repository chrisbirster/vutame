package social

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ErrNotFound        = errors.New("creator not found")
	ErrSelfFollow      = errors.New("cannot follow yourself")
	ErrProfileRequired = errors.New("profile required")
	ErrInvalidMetadata = errors.New("invalid discovery metadata")
	ErrInvalidCursor   = errors.New("invalid discovery cursor")
)

const (
	MaxCategoryLength  = 40
	MaxInterestLength  = 32
	MaxInterests       = 8
	MaxSearchLength    = 80
	DefaultSearchLimit = 24
	MaxSearchLimit     = 50
)

type Creator struct {
	Handle         string   `json:"handle"`
	DisplayName    string   `json:"display_name"`
	Bio            string   `json:"bio"`
	AvatarURL      string   `json:"avatar_url,omitempty"`
	Theme          string   `json:"theme"`
	Verified       bool     `json:"verified"`
	Category       string   `json:"category,omitempty"`
	Interests      []string `json:"interests"`
	FollowerCount  int      `json:"follower_count"`
	FollowingCount int      `json:"following_count"`
	ViewerFollows  bool     `json:"viewer_follows"`
}

type CreatorPage struct {
	Creators   []Creator `json:"creators"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

type MetadataInput struct {
	Category  string   `json:"category"`
	Interests []string `json:"interests"`
}

type SearchInput struct {
	Query    string
	Category string
	Interest string
	Cursor   string
	Limit    int
}

type Store interface {
	Follow(context.Context, string, string) error
	Unfollow(context.Context, string, string) error
	Creator(context.Context, string, string) (Creator, error)
	Followers(context.Context, string, string) ([]Creator, error)
	Following(context.Context, string, string) ([]Creator, error)
	Search(context.Context, SearchInput, string) ([]Creator, error)
	SearchPage(context.Context, SearchInput, string) (CreatorPage, error)
	UpdateMetadata(context.Context, string, MetadataInput) (Creator, error)
}

func ValidateMetadata(input MetadataInput) (MetadataInput, error) {
	input.Category = strings.TrimSpace(input.Category)
	if utf8.RuneCountInString(input.Category) > MaxCategoryLength {
		return MetadataInput{}, fmt.Errorf("%w: category must be %d characters or fewer", ErrInvalidMetadata, MaxCategoryLength)
	}
	if len(input.Interests) > MaxInterests {
		return MetadataInput{}, fmt.Errorf("%w: at most %d interests are allowed", ErrInvalidMetadata, MaxInterests)
	}
	seen := make(map[string]struct{}, len(input.Interests))
	interests := make([]string, 0, len(input.Interests))
	for _, raw := range input.Interests {
		interest := strings.ToLower(strings.TrimSpace(raw))
		if interest == "" {
			continue
		}
		if utf8.RuneCountInString(interest) > MaxInterestLength {
			return MetadataInput{}, fmt.Errorf("%w: interests must be %d characters or fewer", ErrInvalidMetadata, MaxInterestLength)
		}
		if _, exists := seen[interest]; exists {
			continue
		}
		seen[interest] = struct{}{}
		interests = append(interests, interest)
	}
	if len(interests) > MaxInterests {
		return MetadataInput{}, fmt.Errorf("%w: at most %d interests are allowed", ErrInvalidMetadata, MaxInterests)
	}
	input.Interests = interests
	return input, nil
}

func NormalizeSearch(input SearchInput) (SearchInput, error) {
	input.Query = strings.TrimSpace(input.Query)
	input.Category = strings.TrimSpace(input.Category)
	input.Interest = strings.ToLower(strings.TrimSpace(input.Interest))
	input.Cursor = strings.TrimSpace(input.Cursor)
	if utf8.RuneCountInString(input.Query) > MaxSearchLength {
		return SearchInput{}, fmt.Errorf("%w: search query must be %d characters or fewer", ErrInvalidMetadata, MaxSearchLength)
	}
	if utf8.RuneCountInString(input.Category) > MaxCategoryLength {
		return SearchInput{}, fmt.Errorf("%w: category filter is too long", ErrInvalidMetadata)
	}
	if utf8.RuneCountInString(input.Interest) > MaxInterestLength {
		return SearchInput{}, fmt.Errorf("%w: interest filter is too long", ErrInvalidMetadata)
	}
	if input.Limit <= 0 {
		input.Limit = DefaultSearchLimit
	}
	if input.Limit > MaxSearchLimit {
		input.Limit = MaxSearchLimit
	}
	return input, nil
}

func EncodeSearchCursor(offset int) string {
	if offset <= 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func DecodeSearchCursor(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, ErrInvalidCursor
	}
	offset, err := strconv.Atoi(string(decoded))
	if err != nil || offset < 0 || offset > 100000 {
		return 0, ErrInvalidCursor
	}
	return offset, nil
}
