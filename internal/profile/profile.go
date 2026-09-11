package profile

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNotFound     = errors.New("profile not found")
	ErrInvalidHandle = errors.New("invalid handle")
)

const (
	MinHandleLength = 3
	MaxHandleLength = 32
)

var reservedHandles = map[string]struct{}{
	"admin": {}, "api": {}, "create": {}, "discover": {}, "help": {},
	"login": {}, "signup": {}, "support": {}, "vuta": {}, "vutame": {},
}

type Link struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	URL      string `json:"url"`
	Kind     string `json:"kind"`
	Position int    `json:"position"`
	IsActive bool   `json:"is_active"`
}

type Profile struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Verified    bool   `json:"verified"`
	ATProtoDID  string `json:"atproto_did,omitempty"`
	Links       []Link `json:"links"`
}

type Store interface {
	Get(handle string) (Profile, error)
	Discover() ([]Profile, error)
	HandleAvailable(handle string) (bool, error)
}

type MemoryStore struct {
	mu       sync.RWMutex
	byHandle map[string]Profile
}

func NewMemoryStore(items []Profile) *MemoryStore {
	store := &MemoryStore{byHandle: make(map[string]Profile, len(items))}
	for _, item := range items {
		item.Handle = NormalizeHandle(item.Handle)
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
			Links: []Link{
				{ID: "lnk_01_github", Label: "GitHub", URL: "https://github.com/chrisbirster", Kind: "social", Position: 0, IsActive: true},
				{ID: "lnk_02_vutame", Label: "Vutame", URL: "https://vutame.com", Kind: "website", Position: 1, IsActive: true},
			},
		},
	}
}

func NormalizeHandle(handle string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
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
	copy := cloneProfile(item)
	links := copy.Links[:0]
	for _, link := range copy.Links {
		if link.IsActive {
			links = append(links, link)
		}
	}
	copy.Links = links
	sort.SliceStable(copy.Links, func(i, j int) bool {
		if copy.Links[i].Position == copy.Links[j].Position {
			return copy.Links[i].ID < copy.Links[j].ID
		}
		return copy.Links[i].Position < copy.Links[j].Position
	})
	return copy
}

func cloneProfile(item Profile) Profile {
	copy := item
	copy.Links = append([]Link(nil), item.Links...)
	return copy
}
