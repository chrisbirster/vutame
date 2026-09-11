package profile

import (
	"errors"
	"sort"
	"strings"
)

var ErrNotFound = errors.New("profile not found")

type Link struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
}

type Profile struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Verified    bool   `json:"verified"`
	Links       []Link `json:"links"`
}

var seed = map[string]Profile{
	"chrisdontmiss": {
		Handle:      "chrisdontmiss",
		DisplayName: "@chrisdontmiss",
		Bio:         "Building things on the internet.",
		Links: []Link{
			{ID: "github", Label: "GitHub", URL: "https://github.com/chrisbirster", Kind: "social"},
			{ID: "vutame", Label: "Vutame", URL: "https://vutame.com", Kind: "website"},
		},
	},
}

func NormalizeHandle(handle string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
}

func Get(handle string) (Profile, error) {
	handle = NormalizeHandle(handle)
	item, ok := seed[handle]
	if !ok {
		return Profile{}, ErrNotFound
	}
	return item, nil
}

func Discover() []Profile {
	items := make([]Profile, 0, len(seed))
	for _, item := range seed {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Handle < items[j].Handle
	})
	return items
}
