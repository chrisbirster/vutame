package safety

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chrisbirster/vutame/internal/profile"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("safety: database is required")
	}
	for _, table := range []string{"profiles", "follows", "blocks", "mutes", "creator_privacy", "reports", "moderation_profiles"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			return nil, fmt.Errorf("verify safety table %s: %w", table, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("safety schema missing table %s; run Atlas schema apply", table)
		}
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Block(ctx context.Context, actorUserID, handle string) error {
	actorUserID = strings.TrimSpace(actorUserID)
	targetUserID, err := s.targetUserID(ctx, actorUserID, handle)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin block: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO blocks (blocker_user_id, blocked_user_id, created_at) VALUES (?, ?, ?) ON CONFLICT(blocker_user_id, blocked_user_id) DO NOTHING`, actorUserID, targetUserID, now); err != nil {
		return fmt.Errorf("block creator: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM follows WHERE (follower_user_id=? AND following_user_id=?) OR (follower_user_id=? AND following_user_id=?)`, actorUserID, targetUserID, targetUserID, actorUserID); err != nil {
		return fmt.Errorf("remove blocked follows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit block: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Unblock(ctx context.Context, actorUserID, handle string) error {
	targetUserID, err := s.resolveHandle(ctx, handle)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM blocks WHERE blocker_user_id=? AND blocked_user_id=?`, strings.TrimSpace(actorUserID), targetUserID)
	if err != nil {
		return fmt.Errorf("unblock creator: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Mute(ctx context.Context, actorUserID, handle string) error {
	actorUserID = strings.TrimSpace(actorUserID)
	targetUserID, err := s.targetUserID(ctx, actorUserID, handle)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO mutes (muter_user_id, muted_user_id, created_at) VALUES (?, ?, ?) ON CONFLICT(muter_user_id, muted_user_id) DO NOTHING`, actorUserID, targetUserID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("mute creator: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Unmute(ctx context.Context, actorUserID, handle string) error {
	targetUserID, err := s.resolveHandle(ctx, handle)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM mutes WHERE muter_user_id=? AND muted_user_id=?`, strings.TrimSpace(actorUserID), targetUserID)
	if err != nil {
		return fmt.Errorf("unmute creator: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Blocks(ctx context.Context, userID string) ([]Relation, error) {
	return s.relations(ctx, `SELECT p.handle, p.display_name, p.avatar_url FROM blocks b JOIN profiles p ON p.user_id=b.blocked_user_id WHERE b.blocker_user_id=? ORDER BY b.created_at DESC, p.handle`, strings.TrimSpace(userID))
}

func (s *SQLiteStore) Mutes(ctx context.Context, userID string) ([]Relation, error) {
	return s.relations(ctx, `SELECT p.handle, p.display_name, p.avatar_url FROM mutes m JOIN profiles p ON p.user_id=m.muted_user_id WHERE m.muter_user_id=? ORDER BY m.created_at DESC, p.handle`, strings.TrimSpace(userID))
}

func (s *SQLiteStore) Privacy(ctx context.Context, userID string) (Privacy, error) {
	userID = strings.TrimSpace(userID)
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id=?`, userID).Scan(&exists); err != nil {
		return Privacy{}, fmt.Errorf("check privacy profile: %w", err)
	}
	if exists != 1 {
		return Privacy{}, ErrProfileRequired
	}
	var discoverable, activityVisible, allowFollows int
	err := s.db.QueryRowContext(ctx, `SELECT discoverable, activity_visible, allow_follows FROM creator_privacy WHERE user_id=?`, userID).Scan(&discoverable, &activityVisible, &allowFollows)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultPrivacy(), nil
	}
	if err != nil {
		return Privacy{}, fmt.Errorf("load privacy: %w", err)
	}
	return Privacy{Discoverable: discoverable != 0, ActivityVisible: activityVisible != 0, AllowFollows: allowFollows != 0}, nil
}

func (s *SQLiteStore) UpdatePrivacy(ctx context.Context, userID string, input Privacy) (Privacy, error) {
	userID = strings.TrimSpace(userID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Privacy{}, fmt.Errorf("begin privacy update: %w", err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id=?`, userID).Scan(&exists); err != nil {
		return Privacy{}, fmt.Errorf("check privacy owner: %w", err)
	}
	if exists != 1 {
		return Privacy{}, ErrProfileRequired
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO creator_privacy (user_id, discoverable, activity_visible, allow_follows, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET discoverable=excluded.discoverable, activity_visible=excluded.activity_visible, allow_follows=excluded.allow_follows, updated_at=excluded.updated_at`, userID, boolInt(input.Discoverable), boolInt(input.ActivityVisible), boolInt(input.AllowFollows), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return Privacy{}, fmt.Errorf("update privacy: %w", err)
	}
	if !input.AllowFollows {
		if _, err := tx.ExecContext(ctx, `DELETE FROM follows WHERE following_user_id=?`, userID); err != nil {
			return Privacy{}, fmt.Errorf("remove inbound follows: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Privacy{}, fmt.Errorf("commit privacy: %w", err)
	}
	return input, nil
}

func (s *SQLiteStore) Report(ctx context.Context, reporterUserID, handle string, input ReportInput) (Report, error) {
	input, err := ValidateReport(input)
	if err != nil {
		return Report{}, err
	}
	reporterUserID = strings.TrimSpace(reporterUserID)
	targetUserID, err := s.targetUserID(ctx, reporterUserID, handle)
	if err != nil {
		return Report{}, err
	}
	id, err := randomID("rpt_")
	if err != nil {
		return Report{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO reports (id, reporter_user_id, reported_user_id, reason, detail, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'open', ?, ?)`, id, reporterUserID, targetUserID, input.Reason, input.Detail, now, now); err != nil {
		return Report{}, fmt.Errorf("create report: %w", err)
	}
	return Report{ID: id, ReportedHandle: profile.NormalizeHandle(handle), Reason: input.Reason, Detail: input.Detail, Status: "open", CreatedAt: now}, nil
}

func (s *SQLiteStore) SetModeration(ctx context.Context, handle string, input Moderation) error {
	input, err := ValidateModeration(input)
	if err != nil {
		return err
	}
	userID, err := s.resolveHandle(ctx, handle)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO moderation_profiles (user_id, state, note, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT(user_id) DO UPDATE SET state=excluded.state, note=excluded.note, updated_at=excluded.updated_at`, userID, input.State, input.Note, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("set moderation state: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Moderation(ctx context.Context, handle string) (Moderation, error) {
	userID, err := s.resolveHandle(ctx, handle)
	if err != nil {
		return Moderation{}, err
	}
	var item Moderation
	err = s.db.QueryRowContext(ctx, `SELECT state, note FROM moderation_profiles WHERE user_id=?`, userID).Scan(&item.State, &item.Note)
	if errors.Is(err, sql.ErrNoRows) {
		return Moderation{State: "active"}, nil
	}
	if err != nil {
		return Moderation{}, fmt.Errorf("load moderation state: %w", err)
	}
	return item, nil
}

func (s *SQLiteStore) targetUserID(ctx context.Context, actorUserID, handle string) (string, error) {
	actorUserID = strings.TrimSpace(actorUserID)
	var actorExists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id=?`, actorUserID).Scan(&actorExists); err != nil {
		return "", fmt.Errorf("check safety actor: %w", err)
	}
	if actorExists != 1 {
		return "", ErrProfileRequired
	}
	targetUserID, err := s.resolveHandle(ctx, handle)
	if err != nil {
		return "", err
	}
	if actorUserID == targetUserID {
		return "", ErrSelfAction
	}
	return targetUserID, nil
}

func (s *SQLiteStore) resolveHandle(ctx context.Context, handle string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM profiles WHERE handle=?`, profile.NormalizeHandle(handle)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve safety target: %w", err)
	}
	return userID, nil
}

func (s *SQLiteStore) relations(ctx context.Context, query, userID string) ([]Relation, error) {
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Relation, 0)
	for rows.Next() {
		var item Relation
		if err := rows.Scan(&item.Handle, &item.DisplayName, &item.AvatarURL); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func randomID(prefix string) (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}
