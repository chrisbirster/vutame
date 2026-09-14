package operations

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/chrisbirster/vutame/internal/activity"
)

type webhookActivityStore struct {
	base activity.Store
	ops  *Store
}

func WrapActivityStore(base activity.Store, ops *Store) activity.Store {
	if base == nil || ops == nil {
		return base
	}
	return &webhookActivityStore{base: base, ops: ops}
}

func (s *webhookActivityStore) Record(ctx context.Context, userID, kind, linkID, label string) error {
	if err := s.base.Record(ctx, userID, kind, linkID, label); err != nil {
		return err
	}
	event := ""
	switch kind {
	case activity.KindProfileUpdated:
		event = "profile.updated"
	case activity.KindLinkFeatured:
		event = "link.featured"
	}
	if event == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{
		"event": event,
		"link_id": linkID,
		"label": label,
	})
	if err := s.ops.QueueWebhookEvent(ctx, userID, event, string(payload)); err != nil {
		slog.Warn("queue webhook event", "event", event, "error", err)
	}
	return nil
}

func (s *webhookActivityStore) Following(ctx context.Context, userID, cursor string, limit int) (activity.Page, error) {
	return s.base.Following(ctx, userID, cursor, limit)
}

func (s *webhookActivityStore) Recent(ctx context.Context, cursor string, limit int) (activity.Page, error) {
	return s.base.Recent(ctx, cursor, limit)
}

func (s *webhookActivityStore) Trending(ctx context.Context, limit int) ([]activity.Trend, error) {
	return s.base.Trending(ctx, limit)
}
