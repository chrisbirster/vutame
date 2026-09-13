package profile

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound       = errors.New("profile not found")
	ErrInvalidHandle  = errors.New("invalid handle")
	ErrHandleTaken    = errors.New("handle already claimed")
	ErrProfileExists  = errors.New("profile already exists")
	ErrInvalidProfile = errors.New("invalid profile")
	ErrInvalidLink    = errors.New("invalid link")
)

const (
	MinHandleLength = 3
	MaxHandleLength = 32
	DefaultTheme    = "midnight"
	DefaultLinkKind = "website"
)

var reservedHandles = map[string]struct{}{
	"admin": {}, "api": {}, "create": {}, "discover": {}, "help": {},
	"login": {}, "me": {}, "settings": {}, "signin": {}, "signup": {},
	"support": {}, "vuta": {}, "vutame": {},
}

var supportedThemes = map[string]struct{}{
	"midnight": {},
	"paper":    {},
	"neon":     {},
	"forest":   {},
}

var supportedLinkKinds = map[string]struct{}{
	"website":    {},
	"project":    {},
	"github":     {},
	"youtube":    {},
	"instagram":  {},
	"tiktok":     {},
	"x":          {},
	"bluesky":    {},
	"linkedin":   {},
	"spotify":    {},
	"newsletter": {},
	"shop":       {},
	"social":     {},
}

type Link struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	URL          string `json:"url"`
	Kind         string `json:"kind"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Featured     bool   `json:"featured"`
	VisibleFrom  string `json:"visible_from,omitempty"`
	VisibleUntil string `json:"visible_until,omitempty"`
	Position     int    `json:"position"`
	IsActive     bool   `json:"is_active"`
}

type Profile struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Theme       string `json:"theme"`
	Verified    bool   `json:"verified"`
	ATProtoDID  string `json:"atproto_did,omitempty"`
	Links       []Link `json:"links"`
}

type UpdateInput struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url"`
	Theme       string `json:"theme"`
}

type LinkInput struct {
	Label        string `json:"label"`
	URL          string `json:"url"`
	Kind         string `json:"kind"`
	ThumbnailURL string `json:"thumbnail_url"`
	Featured     bool   `json:"featured"`
	VisibleFrom  string `json:"visible_from"`
	VisibleUntil string `json:"visible_until"`
	IsActive     bool   `json:"is_active"`
}

type Store interface {
	Get(handle string) (Profile, error)
	Discover() ([]Profile, error)
	HandleAvailable(handle string) (bool, error)
}

type Editor interface {
	GetOwned(context.Context, string) (Profile, error)
	Claim(context.Context, string, string) (Profile, error)
	Update(context.Context, string, UpdateInput) (Profile, error)
	CreateLink(context.Context, string, LinkInput) (Link, error)
	UpdateLink(context.Context, string, string, LinkInput) (Link, error)
	DeleteLink(context.Context, string, string) error
	ReorderLinks(context.Context, string, []string) error
}

type MemoryStore struct {
	mu       sync.RWMutex
	byHandle map[string]Profile
}

func NewMemoryStore(items []Profile) *MemoryStore {
	store := &MemoryStore{byHandle: make(map[string]Profile, len(items))}
	for _, item := range items {
		item.Handle = NormalizeHandle(item.Handle)
		item.Theme = NormalizeTheme(item.Theme)
		for index := range item.Links {
			item.Links[index].Kind = NormalizeLinkKind(item.Links[index].Kind)
		}
		store.byHandle[item.Handle] = cloneProfile(item)
	}
	return store
}

func NewSeedStore() *MemoryStore {
	return NewMemoryStore(SeedProfiles())
}

func SeedProfiles() []Profile {
	return []Profile{
		{
			ID:          "usr_01_chrisdontmiss",
			Handle:      "chrisdontmiss",
			DisplayName: "@chrisdontmiss",
			Bio:         "Building things on the internet.",
			Theme:       DefaultTheme,
			Links: []Link{
				{ID: "lnk_01_github", Label: "GitHub", URL: "https://github.com/chrisbirster", Kind: "github", Position: 0, IsActive: true},
				{ID: "lnk_02_vutame", Label: "Vutame", URL: "https://vutame.com", Kind: "website", Position: 1, IsActive: true},
			},
		},
	}
}

func NormalizeHandle(handle string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
}

func NormalizeTheme(theme string) string {
	theme = strings.ToLower(strings.TrimSpace(theme))
	if theme == "" {
		return DefaultTheme
	}
	return theme
}

func NormalizeLinkKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return DefaultLinkKind
	}
	return kind
}

func ValidateTheme(theme string) (string, error) {
	theme = NormalizeTheme(theme)
	if _, ok := supportedThemes[theme]; !ok {
		return "", fmt.Errorf("%w: unsupported theme %q", ErrInvalidProfile, theme)
	}
	return theme, nil
}

func ValidateLinkKind(kind string) (string, error) {
	kind = NormalizeLinkKind(kind)
	if _, ok := supportedLinkKinds[kind]; !ok {
		return "", fmt.Errorf("%w: unsupported link kind %q", ErrInvalidLink, kind)
	}
	return kind, nil
}

func ValidateHandle(handle string) error {
	handle = NormalizeHandle(handle)
	if len(handle) < MinHandleLength || len(handle) > MaxHandleLength {
		return fmt.Errorf("%w: handle must be %d-%d characters", ErrInvalidHandle, MinHandleLength, MaxHandleLength)
	}
	for index := 0; index < len(handle); index++ {
		char := handle[index]
		alphanumeric := char >= 'a' && char <= 'z' || char >= '0' && char <= '9'
		punctuation := char == '.' || char == '_' || char == '-'
		if !alphanumeric && !punctuation {
			return fmt.Errorf("%w: handle contains unsupported characters", ErrInvalidHandle)
		}
		if (index == 0 || index == len(handle)-1) && !alphanumeric {
			return fmt.Errorf("%w: handle must start and end with a letter or number", ErrInvalidHandle)
		}
	}
	if _, reserved := reservedHandles[handle]; reserved {
		return fmt.Errorf("%w: handle is reserved", ErrInvalidHandle)
	}
	return nil
}

func ValidateUpdateInput(input UpdateInput) (UpdateInput, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Bio = strings.TrimSpace(input.Bio)
	input.AvatarURL = strings.TrimSpace(input.AvatarURL)
	if len(input.DisplayName) > 80 {
		return UpdateInput{}, fmt.Errorf("%w: display name must be 80 characters or fewer", ErrInvalidProfile)
	}
	if len(input.Bio) > 320 {
		return UpdateInput{}, fmt.Errorf("%w: bio must be 320 characters or fewer", ErrInvalidProfile)
	}
	if err := validateOptionalHTTPURL(input.AvatarURL, "avatar URL", ErrInvalidProfile); err != nil {
		return UpdateInput{}, err
	}
	theme, err := ValidateTheme(input.Theme)
	if err != nil {
		return UpdateInput{}, err
	}
	input.Theme = theme
	return input, nil
}

func ValidateLinkInput(input LinkInput) (LinkInput, error) {
	input.Label = strings.TrimSpace(input.Label)
	input.URL = strings.TrimSpace(input.URL)
	input.ThumbnailURL = strings.TrimSpace(input.ThumbnailURL)
	input.VisibleFrom = strings.TrimSpace(input.VisibleFrom)
	input.VisibleUntil = strings.TrimSpace(input.VisibleUntil)
	if input.Label == "" || len(input.Label) > 100 {
		return LinkInput{}, fmt.Errorf("%w: label must be 1-100 characters", ErrInvalidLink)
	}
	if len(input.URL) > 2048 {
		return LinkInput{}, fmt.Errorf("%w: URL is too long", ErrInvalidLink)
	}
	if err := validateRequiredHTTPURL(input.URL, "URL", ErrInvalidLink); err != nil {
		return LinkInput{}, err
	}
	kind, err := ValidateLinkKind(input.Kind)
	if err != nil {
		return LinkInput{}, err
	}
	input.Kind = kind
	if len(input.ThumbnailURL) > 2048 {
		return LinkInput{}, fmt.Errorf("%w: thumbnail URL is too long", ErrInvalidLink)
	}
	if err := validateOptionalHTTPURL(input.ThumbnailURL, "thumbnail URL", ErrInvalidLink); err != nil {
		return LinkInput{}, err
	}
	from, err := normalizeOptionalTimestamp(input.VisibleFrom, "visible_from")
	if err != nil {
		return LinkInput{}, err
	}
	until, err := normalizeOptionalTimestamp(input.VisibleUntil, "visible_until")
	if err != nil {
		return LinkInput{}, err
	}
	input.VisibleFrom = from
	input.VisibleUntil = until
	if from != "" && until != "" {
		fromTime, _ := time.Parse(time.RFC3339, from)
		untilTime, _ := time.Parse(time.RFC3339, until)
		if !untilTime.After(fromTime) {
			return LinkInput{}, fmt.Errorf("%w: visible_until must be after visible_from", ErrInvalidLink)
		}
	}
	return input, nil
}

func normalizeOptionalTimestamp(value, field string) (string, error) {
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", fmt.Errorf("%w: %s must be an RFC3339 timestamp", ErrInvalidLink, field)
	}
	return parsed.UTC().Format(time.RFC3339), nil
}

func validateRequiredHTTPURL(value, field string, sentinel error) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s must be an absolute http(s) URL", sentinel, field)
	}
	return validateOptionalHTTPURL(value, field, sentinel)
}

func validateOptionalHTTPURL(value, field string, sentinel error) error {
	if value == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%w: %s must be an absolute http(s) URL", sentinel, field)
	}
	return nil
}

func (s *MemoryStore) Get(handle string) (Profile, error) {
	handle = NormalizeHandle(handle)
	s.mu.RLock()
	item, ok := s.byHandle[handle]
	s.mu.RUnlock()
	if !ok {
		return Profile{}, ErrNotFound
	}
	return publicProfile(item), nil
}

func (s *MemoryStore) Discover() ([]Profile, error) {
	s.mu.RLock()
	items := make([]Profile, 0, len(s.byHandle))
	for _, item := range s.byHandle {
		items = append(items, publicProfile(item))
	}
	s.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool {
		return items[i].Handle < items[j].Handle
	})
	return items, nil
}

func (s *MemoryStore) HandleAvailable(handle string) (bool, error) {
	if err := ValidateHandle(handle); err != nil {
		return false, err
	}
	handle = NormalizeHandle(handle)
	s.mu.RLock()
	_, exists := s.byHandle[handle]
	s.mu.RUnlock()
	return !exists, nil
}

func publicProfile(item Profile) Profile {
	return publicProfileAt(item, time.Now().UTC())
}

func publicProfileAt(item Profile, now time.Time) Profile {
	copy := cloneProfile(item)
	copy.Theme = NormalizeTheme(copy.Theme)
	links := copy.Links[:0]
	for _, link := range copy.Links {
		if link.IsActive && linkVisibleAt(link, now) {
			link.Kind = NormalizeLinkKind(link.Kind)
			links = append(links, link)
		}
	}
	copy.Links = links
	sort.SliceStable(copy.Links, func(i, j int) bool {
		if copy.Links[i].Featured != copy.Links[j].Featured {
			return copy.Links[i].Featured
		}
		if copy.Links[i].Position == copy.Links[j].Position {
			return copy.Links[i].ID < copy.Links[j].ID
		}
		return copy.Links[i].Position < copy.Links[j].Position
	})
	return copy
}

func linkVisibleAt(link Link, now time.Time) bool {
	if link.VisibleFrom != "" {
		from, err := time.Parse(time.RFC3339Nano, link.VisibleFrom)
		if err != nil || now.Before(from) {
			return false
		}
	}
	if link.VisibleUntil != "" {
		until, err := time.Parse(time.RFC3339Nano, link.VisibleUntil)
		if err != nil || !now.Before(until) {
			return false
		}
	}
	return true
}

func cloneProfile(item Profile) Profile {
	copy := item
	copy.Links = append([]Link(nil), item.Links...)
	return copy
}
