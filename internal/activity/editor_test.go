package activity

import (
	"context"
	"testing"

	"github.com/chrisbirster/vutame/internal/profile"
)

type recordedEvent struct{ kind, linkID, label string }

type captureStore struct{ events []recordedEvent }

func (s *captureStore) Record(_ context.Context, _ string, kind, linkID, label string) error {
	s.events = append(s.events, recordedEvent{kind: kind, linkID: linkID, label: label})
	return nil
}
func (*captureStore) Following(context.Context, string, string, int) (Page, error) { return Page{}, nil }
func (*captureStore) Recent(context.Context, string, int) (Page, error)            { return Page{}, nil }
func (*captureStore) Trending(context.Context, int) ([]Trend, error)               { return nil, nil }

type fakeEditor struct{ owned profile.Profile }

func (e *fakeEditor) GetOwned(context.Context, string) (profile.Profile, error) { return e.owned, nil }
func (e *fakeEditor) Claim(context.Context, string, string) (profile.Profile, error) { return e.owned, nil }
func (e *fakeEditor) Update(_ context.Context, _ string, input profile.UpdateInput) (profile.Profile, error) {
	e.owned.DisplayName = input.DisplayName
	return e.owned, nil
}
func (e *fakeEditor) CreateLink(_ context.Context, _ string, input profile.LinkInput) (profile.Link, error) {
	item := profile.Link{ID: "lnk_new", Label: input.Label, URL: input.URL, Kind: input.Kind, Featured: input.Featured, IsActive: input.IsActive}
	e.owned.Links = append(e.owned.Links, item)
	return item, nil
}
func (e *fakeEditor) UpdateLink(_ context.Context, _ string, linkID string, input profile.LinkInput) (profile.Link, error) {
	for index := range e.owned.Links {
		if e.owned.Links[index].ID == linkID {
			e.owned.Links[index].Label = input.Label
			e.owned.Links[index].Featured = input.Featured
			e.owned.Links[index].IsActive = input.IsActive
			return e.owned.Links[index], nil
		}
	}
	return profile.Link{}, profile.ErrNotFound
}
func (*fakeEditor) DeleteLink(context.Context, string, string) error       { return nil }
func (*fakeEditor) ReorderLinks(context.Context, string, []string) error  { return nil }

func TestRecordingEditorRecordsMeaningfulTransitions(t *testing.T) {
	delegate := &fakeEditor{owned: profile.Profile{ID: "usr_one", Handle: "one", DisplayName: "One", Links: []profile.Link{{ID: "lnk_1", Label: "Old", Featured: false, IsActive: true}}}}
	capture := &captureStore{}
	editor := NewRecordingEditor(delegate, capture)
	ctx := context.Background()

	if _, err := editor.Update(ctx, "usr_one", profile.UpdateInput{DisplayName: "One Updated"}); err != nil {
		t.Fatal(err)
	}
	if _, err := editor.UpdateLink(ctx, "usr_one", "lnk_1", profile.LinkInput{Label: "Featured", Featured: true, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := editor.UpdateLink(ctx, "usr_one", "lnk_1", profile.LinkInput{Label: "Featured Again", Featured: true, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := editor.CreateLink(ctx, "usr_one", profile.LinkInput{Label: "Fresh feature", Featured: true, IsActive: true}); err != nil {
		t.Fatal(err)
	}

	if len(capture.events) != 3 {
		t.Fatalf("events = %#v", capture.events)
	}
	if capture.events[0].kind != KindProfileUpdated {
		t.Fatalf("first event = %#v", capture.events[0])
	}
	if capture.events[1].kind != KindLinkFeatured || capture.events[1].linkID != "lnk_1" {
		t.Fatalf("feature event = %#v", capture.events[1])
	}
	if capture.events[2].kind != KindLinkFeatured || capture.events[2].linkID != "lnk_new" {
		t.Fatalf("create feature event = %#v", capture.events[2])
	}
}
