package safety

import "github.com/chrisbirster/vutame/internal/moderation"

// ModerationOperations exposes the privileged moderation service to the HTTP
// composition root without broadening the end-user safety Store interface.
// SQLiteStore already owns the same durable database handle, so both safety
// and privileged moderation policies observe the same transactionally applied
// state.
func (s *SQLiteStore) ModerationOperations() *moderation.Service {
	service, err := moderation.NewService(s.db)
	if err != nil {
		return nil
	}
	return service
}
