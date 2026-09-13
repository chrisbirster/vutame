package profile

import (
	"errors"
	"testing"
)

func TestNormalizeHandle(t *testing.T) {
	if got := NormalizeHandle("  @ChrisDontMiss  "); got != "chrisdontmiss" {
		t.Fatalf("NormalizeHandle() = %q, want %q", got, "chrisdontmiss")
	}
}

func TestValidateHandle(t *testing.T) {
	valid := []string{"chrisdontmiss", "chris.dev", "user_name", "user-name", "abc123"}
	for _, handle := range valid {
		if err := ValidateHandle(handle); err != nil {
			t.Errorf("ValidateHandle(%q) unexpected error: %v", handle, err)
		}
	}

	invalid := []string{"ab", "_chris", "chris_", "Chris!", "api", "this-handle-is-far-too-long-to-be-accepted-by-vutame"}
	for _, handle := range invalid {
		if err := ValidateHandle(handle); !errors.Is(err, ErrInvalidHandle) {
			t.Errorf("ValidateHandle(%q) error = %v, want ErrInvalidHandle", handle, err)
		}
	}
}

func TestMemoryStorePublicProfile(t *testing.T) {
	store := NewMemoryStore([]Profile{{
		ID: "usr_1", Handle: "tester", DisplayName: "Tester",
		Links: []Link{
			{ID: "b", Label: "Second", URL: "https://example.com/2", Position: 2, IsActive: true},
			{ID: "hidden", Label: "Hidden", URL: "https://example.com/hidden", Position: 1, IsActive: false},
			{ID: "a", Label: "First", URL: "https://example.com/1", Position: 0, IsActive: true},
		},
	}})

	item, err := store.Get("@TESTER")
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "usr_1" {
		t.Fatalf("ID = %q, want stable user ID", item.ID)
	}
	if len(item.Links) != 2 || item.Links[0].ID != "a" || item.Links[1].ID != "b" {
		t.Fatalf("public links = %#v, want active links in position order", item.Links)
	}
}

func TestHandleAvailable(t *testing.T) {
	store := NewSeedStore()
	available, err := store.HandleAvailable("newcreator")
	if err != nil || !available {
		t.Fatalf("newcreator available = %v, err = %v; want true, nil", available, err)
	}
	available, err = store.HandleAvailable("chrisdontmiss")
	if err != nil || available {
		t.Fatalf("chrisdontmiss available = %v, err = %v; want false, nil", available, err)
	}
	if _, err := store.HandleAvailable("api"); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("reserved handle error = %v, want ErrInvalidHandle", err)
	}
}
