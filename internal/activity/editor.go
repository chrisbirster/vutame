package activity

import (
	"context"
	"log/slog"

	"github.com/chrisbirster/vutame/internal/profile"
)

type RecordingEditor struct {
	delegate profile.Editor
	activity Store
}

func NewRecordingEditor(delegate profile.Editor, activity Store) profile.Editor {
	if delegate == nil || activity == nil {
		return delegate
	}
	return &RecordingEditor{delegate: delegate, activity: activity}
}

func (e *RecordingEditor) GetOwned(ctx context.Context, userID string) (profile.Profile, error) {
	return e.delegate.GetOwned(ctx, userID)
}

func (e *RecordingEditor) Claim(ctx context.Context, userID, handle string) (profile.Profile, error) {
	return e.delegate.Claim(ctx, userID, handle)
}

func (e *RecordingEditor) Update(ctx context.Context, userID string, input profile.UpdateInput) (profile.Profile, error) {
	item, err := e.delegate.Update(ctx, userID, input)
	if err != nil {
		return item, err
	}
	e.record(ctx, userID, KindProfileUpdated, "", item.DisplayName)
	return item, nil
}

func (e *RecordingEditor) CreateLink(ctx context.Context, userID string, input profile.LinkInput) (profile.Link, error) {
	item, err := e.delegate.CreateLink(ctx, userID, input)
	if err != nil {
		return item, err
	}
	if item.Featured && item.IsActive {
		e.record(ctx, userID, KindLinkFeatured, item.ID, item.Label)
	}
	return item, nil
}

func (e *RecordingEditor) UpdateLink(ctx context.Context, userID, linkID string, input profile.LinkInput) (profile.Link, error) {
	wasFeatured := false
	if owned, err := e.delegate.GetOwned(ctx, userID); err == nil {
		for _, link := range owned.Links {
			if link.ID == linkID {
				wasFeatured = link.Featured
				break
			}
		}
	}
	item, err := e.delegate.UpdateLink(ctx, userID, linkID, input)
	if err != nil {
		return item, err
	}
	if item.Featured && item.IsActive && !wasFeatured {
		e.record(ctx, userID, KindLinkFeatured, item.ID, item.Label)
	}
	return item, nil
}

func (e *RecordingEditor) DeleteLink(ctx context.Context, userID, linkID string) error {
	return e.delegate.DeleteLink(ctx, userID, linkID)
}

func (e *RecordingEditor) ReorderLinks(ctx context.Context, userID string, ids []string) error {
	return e.delegate.ReorderLinks(ctx, userID, ids)
}

func (e *RecordingEditor) record(ctx context.Context, userID, kind, linkID, label string) {
	if err := e.activity.Record(ctx, userID, kind, linkID, label); err != nil {
		slog.Warn("record public activity", "kind", kind, "error", err)
	}
}
