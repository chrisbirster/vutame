package moderation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	db  *sql.DB
	now func() time.Time
}

func NewService(db *sql.DB) (*Service, error) {
	if db == nil {
		return nil, errors.New("moderation: database is required")
	}
	for _, table := range []string{
		"users", "profiles", "reports", "moderation_profiles", "moderation_admins",
		"report_workflow", "moderation_actions", "content_takedowns", "moderation_appeals",
	} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			return nil, fmt.Errorf("verify moderation table %s: %w", table, err)
		}
		if count != 1 {
			return nil, fmt.Errorf("moderation schema missing table %s; run Atlas schema apply", table)
		}
	}
	return &Service{db: db, now: time.Now}, nil
}

func (s *Service) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *Service) Role(ctx context.Context, userID string) (Role, error) {
	return requireRole(ctx, s.db, userID)
}

func (s *Service) Queue(ctx context.Context, actorUserID, status string, limit int) ([]Report, error) {
	if _, err := requireRole(ctx, s.db, actorUserID); err != nil {
		return nil, err
	}
	status, err := normalizeQueueStatus(status)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)
	query := `
		SELECT r.id, reporter.handle, reported.handle, r.reason, r.detail, r.status,
		       COALESCE(w.assigned_admin_user_id,''), r.created_at, r.updated_at
		FROM reports r
		JOIN profiles reporter ON reporter.user_id=r.reporter_user_id
		JOIN profiles reported ON reported.user_id=r.reported_user_id
		LEFT JOIN report_workflow w ON w.report_id=r.id
	`
	args := make([]any, 0, 2)
	if status == "" {
		query += ` WHERE r.status IN ('open','reviewing')`
	} else {
		query += ` WHERE r.status=?`
		args = append(args, status)
	}
	query += ` ORDER BY r.created_at ASC, r.id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list moderation reports: %w", err)
	}
	defer rows.Close()
	items := make([]Report, 0)
	for rows.Next() {
		var item Report
		if err := rows.Scan(
			&item.ID, &item.ReporterHandle, &item.ReportedHandle, &item.Reason, &item.Detail,
			&item.Status, &item.AssignedAdminUserID, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan moderation report: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Service) Assign(ctx context.Context, actorUserID, reportID string) (Report, error) {
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return Report{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback()
	role, err := requireRole(ctx, tx, actorUserID)
	if err != nil {
		return Report{}, err
	}
	targetUserID, err := reportTarget(ctx, tx, reportID)
	if err != nil {
		return Report{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE reports SET status='reviewing', updated_at=? WHERE id=? AND status IN ('open','reviewing')`, now, reportID)
	if err != nil {
		return Report{}, fmt.Errorf("assign moderation report: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return Report{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO report_workflow(report_id,assigned_admin_user_id,resolved_at,updated_at)
		VALUES(?,?,NULL,?)
		ON CONFLICT(report_id) DO UPDATE SET assigned_admin_user_id=excluded.assigned_admin_user_id, updated_at=excluded.updated_at
	`, reportID, strings.TrimSpace(actorUserID), now); err != nil {
		return Report{}, fmt.Errorf("persist report assignment: %w", err)
	}
	if err := insertAction(ctx, tx, role, actorUserID, targetUserID, reportID, "assign", "", now); err != nil {
		return Report{}, err
	}
	if err := tx.Commit(); err != nil {
		return Report{}, err
	}
	return s.report(ctx, reportID)
}

func (s *Service) AddNote(ctx context.Context, actorUserID, reportID, note string) error {
	note, err := validateNote(note, 2000)
	if err != nil || note == "" {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	role, err := requireRole(ctx, tx, actorUserID)
	if err != nil {
		return err
	}
	targetUserID, err := reportTarget(ctx, tx, strings.TrimSpace(reportID))
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if err := insertAction(ctx, tx, role, actorUserID, targetUserID, reportID, "note", note, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reports SET updated_at=? WHERE id=?`, now, reportID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Resolve(ctx context.Context, actorUserID, reportID, status, note string) (Report, error) {
	status, err := normalizeResolutionStatus(status)
	if err != nil {
		return Report{}, err
	}
	note, err = validateNote(note, 2000)
	if err != nil {
		return Report{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback()
	role, err := requireRole(ctx, tx, actorUserID)
	if err != nil {
		return Report{}, err
	}
	targetUserID, err := reportTarget(ctx, tx, strings.TrimSpace(reportID))
	if err != nil {
		return Report{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE reports SET status=?, updated_at=? WHERE id=? AND status IN ('open','reviewing')`, status, now, reportID)
	if err != nil {
		return Report{}, fmt.Errorf("resolve moderation report: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return Report{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO report_workflow(report_id,assigned_admin_user_id,resolved_at,updated_at)
		VALUES(?,?,?,?)
		ON CONFLICT(report_id) DO UPDATE SET assigned_admin_user_id=excluded.assigned_admin_user_id, resolved_at=excluded.resolved_at, updated_at=excluded.updated_at
	`, reportID, strings.TrimSpace(actorUserID), now, now); err != nil {
		return Report{}, fmt.Errorf("persist report resolution: %w", err)
	}
	if err := insertAction(ctx, tx, role, actorUserID, targetUserID, reportID, status, note, now); err != nil {
		return Report{}, err
	}
	if err := tx.Commit(); err != nil {
		return Report{}, err
	}
	return s.report(ctx, reportID)
}

func (s *Service) SetState(ctx context.Context, actorUserID, handle, state, note string) error {
	state, err := normalizeModerationState(state)
	if err != nil {
		return err
	}
	note, err = validateNote(note, 2000)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	role, err := requireRole(ctx, tx, actorUserID)
	if err != nil {
		return err
	}
	targetUserID, err := resolveHandle(ctx, tx, handle)
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	storedState := state
	action := state
	if state == "takedown" {
		storedState = "suspended"
	}
	if state == "active" {
		action = "restore"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO moderation_profiles(user_id,state,note,updated_at) VALUES(?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET state=excluded.state,note=excluded.note,updated_at=excluded.updated_at
	`, targetUserID, storedState, note, now); err != nil {
		return fmt.Errorf("set moderation profile state: %w", err)
	}
	actionID, err := insertActionID(ctx, tx, role, actorUserID, targetUserID, "", action, note, now)
	if err != nil {
		return err
	}
	if state == "takedown" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO content_takedowns(user_id,active,reason,action_id,created_at,updated_at)
			VALUES(?,1,?,?,?,?)
			ON CONFLICT(user_id) DO UPDATE SET active=1,reason=excluded.reason,action_id=excluded.action_id,updated_at=excluded.updated_at
		`, targetUserID, note, actionID, now, now); err != nil {
			return fmt.Errorf("activate content takedown: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE content_takedowns SET active=0,updated_at=? WHERE user_id=? AND active=1`, now, targetUserID); err != nil {
			return fmt.Errorf("clear content takedown: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Service) Policy(ctx context.Context, handle string) (PublicPolicy, error) {
	handle = normalizeHandle(handle)
	var state string
	var takedown int
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(m.state,'active'), COALESCE(t.active,0)
		FROM profiles p
		LEFT JOIN moderation_profiles m ON m.user_id=p.user_id
		LEFT JOIN content_takedowns t ON t.user_id=p.user_id
		WHERE p.handle=?
	`, handle).Scan(&state, &takedown)
	if errors.Is(err, sql.ErrNoRows) {
		return PublicPolicy{}, ErrNotFound
	}
	if err != nil {
		return PublicPolicy{}, err
	}
	return PublicPolicy{Local: true, State: state, TakenDown: takedown != 0}, nil
}

func (s *Service) PolicyForDID(ctx context.Context, did string) (PublicPolicy, error) {
	did = strings.TrimSpace(did)
	var state string
	var takedown int
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(m.state,'active'), COALESCE(t.active,0)
		FROM profiles p
		LEFT JOIN moderation_profiles m ON m.user_id=p.user_id
		LEFT JOIN content_takedowns t ON t.user_id=p.user_id
		WHERE p.atproto_did=?
	`, did).Scan(&state, &takedown)
	if errors.Is(err, sql.ErrNoRows) {
		return PublicPolicy{Local: false, State: "active"}, nil
	}
	if err != nil {
		return PublicPolicy{}, err
	}
	return PublicPolicy{Local: true, State: state, TakenDown: takedown != 0}, nil
}

func (s *Service) CreateAppeal(ctx context.Context, userID, message string) (Appeal, error) {
	message, err := validateNote(message, 3000)
	if err != nil || message == "" {
		return Appeal{}, ErrInvalidInput
	}
	userID = strings.TrimSpace(userID)
	var handle string
	if err := s.db.QueryRowContext(ctx, `SELECT handle FROM profiles WHERE user_id=?`, userID).Scan(&handle); errors.Is(err, sql.ErrNoRows) {
		return Appeal{}, ErrProfileRequired
	} else if err != nil {
		return Appeal{}, err
	}
	id, err := randomID("apl_")
	if err != nil {
		return Appeal{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO moderation_appeals(id,user_id,message,status,assigned_admin_user_id,response_note,created_at,updated_at)
		VALUES(?,?,?,'open',NULL,'',?,?)
	`, id, userID, message, now, now); err != nil {
		return Appeal{}, err
	}
	return Appeal{ID: id, Handle: handle, Message: message, Status: "open", CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) AppealsForUser(ctx context.Context, userID string, limit int) ([]Appeal, error) {
	limit = normalizeLimit(limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id,p.handle,a.message,a.status,COALESCE(a.assigned_admin_user_id,''),a.response_note,a.created_at,a.updated_at
		FROM moderation_appeals a JOIN profiles p ON p.user_id=a.user_id
		WHERE a.user_id=? ORDER BY a.created_at DESC,a.id DESC LIMIT ?
	`, strings.TrimSpace(userID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAppeals(rows)
}

func (s *Service) AppealQueue(ctx context.Context, actorUserID, status string, limit int) ([]Appeal, error) {
	if _, err := requireRole(ctx, s.db, actorUserID); err != nil {
		return nil, err
	}
	status, err := normalizeAppealStatus(status)
	if err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)
	query := `
		SELECT a.id,p.handle,a.message,a.status,COALESCE(a.assigned_admin_user_id,''),a.response_note,a.created_at,a.updated_at
		FROM moderation_appeals a JOIN profiles p ON p.user_id=a.user_id
	`
	args := make([]any, 0, 2)
	if status == "" {
		query += ` WHERE a.status IN ('open','reviewing')`
	} else {
		query += ` WHERE a.status=?`
		args = append(args, status)
	}
	query += ` ORDER BY a.created_at ASC,a.id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAppeals(rows)
}

func (s *Service) ReviewAppeal(ctx context.Context, actorUserID, appealID, status, note string) (Appeal, error) {
	status, err := normalizeAppealStatus(status)
	if err != nil || status == "" || status == "open" {
		return Appeal{}, ErrInvalidInput
	}
	note, err = validateNote(note, 2000)
	if err != nil {
		return Appeal{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Appeal{}, err
	}
	defer tx.Rollback()
	role, err := requireRole(ctx, tx, actorUserID)
	if err != nil {
		return Appeal{}, err
	}
	var targetUserID string
	if err := tx.QueryRowContext(ctx, `SELECT user_id FROM moderation_appeals WHERE id=?`, strings.TrimSpace(appealID)).Scan(&targetUserID); errors.Is(err, sql.ErrNoRows) {
		return Appeal{}, ErrNotFound
	} else if err != nil {
		return Appeal{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE moderation_appeals SET status=?,assigned_admin_user_id=?,response_note=?,updated_at=? WHERE id=?
	`, status, strings.TrimSpace(actorUserID), note, now, appealID); err != nil {
		return Appeal{}, err
	}
	action := "appeal_review"
	if status == "resolved" || status == "dismissed" {
		action = "appeal_resolve"
	}
	if err := insertAction(ctx, tx, role, actorUserID, targetUserID, "", action, note, now); err != nil {
		return Appeal{}, err
	}
	if err := tx.Commit(); err != nil {
		return Appeal{}, err
	}
	return s.appeal(ctx, appealID)
}

func (s *Service) Actions(ctx context.Context, actorUserID string, limit int) ([]Action, error) {
	if _, err := requireRole(ctx, s.db, actorUserID); err != nil {
		return nil, err
	}
	limit = normalizeLimit(limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id,COALESCE(a.actor_user_id,''),a.actor_role,COALESCE(p.handle,''),COALESCE(a.report_id,''),a.action,a.note,a.created_at
		FROM moderation_actions a
		LEFT JOIN profiles p ON p.user_id=a.target_user_id
		ORDER BY a.created_at DESC,a.id DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Action, 0)
	for rows.Next() {
		var item Action
		if err := rows.Scan(&item.ID, &item.ActorUserID, &item.ActorRole, &item.TargetHandle, &item.ReportID, &item.Action, &item.Note, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) Stats(ctx context.Context, actorUserID string, since time.Time) (AbuseStats, error) {
	if _, err := requireRole(ctx, s.db, actorUserID); err != nil {
		return AbuseStats{}, err
	}
	if since.IsZero() {
		since = s.now().UTC().Add(-24 * time.Hour)
	}
	stats := AbuseStats{WindowStart: since.UTC().Format(time.RFC3339Nano), ReportsByReason: map[string]int{}}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports WHERE created_at>=?`, stats.WindowStart).Scan(&stats.Reports); err != nil {
		return AbuseStats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports WHERE status='open'`).Scan(&stats.OpenReports); err != nil {
		return AbuseStats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reports WHERE status='reviewing'`).Scan(&stats.ReviewingReports); err != nil {
		return AbuseStats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM moderation_profiles WHERE state='restricted'`).Scan(&stats.ActiveRestrictions); err != nil {
		return AbuseStats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM moderation_profiles WHERE state='suspended'`).Scan(&stats.ActiveSuspensions); err != nil {
		return AbuseStats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_takedowns WHERE active=1`).Scan(&stats.ActiveTakedowns); err != nil {
		return AbuseStats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM moderation_appeals WHERE status IN ('open','reviewing')`).Scan(&stats.OpenAppeals); err != nil {
		return AbuseStats{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT reason,COUNT(*) FROM reports WHERE created_at>=? GROUP BY reason ORDER BY reason`, stats.WindowStart)
	if err != nil {
		return AbuseStats{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var reason string
		var count int
		if err := rows.Scan(&reason, &count); err != nil {
			return AbuseStats{}, err
		}
		stats.ReportsByReason[reason] = count
	}
	return stats, rows.Err()
}

func (s *Service) report(ctx context.Context, reportID string) (Report, error) {
	var item Report
	err := s.db.QueryRowContext(ctx, `
		SELECT r.id,reporter.handle,reported.handle,r.reason,r.detail,r.status,
		       COALESCE(w.assigned_admin_user_id,''),r.created_at,r.updated_at
		FROM reports r
		JOIN profiles reporter ON reporter.user_id=r.reporter_user_id
		JOIN profiles reported ON reported.user_id=r.reported_user_id
		LEFT JOIN report_workflow w ON w.report_id=r.id
		WHERE r.id=?
	`, strings.TrimSpace(reportID)).Scan(
		&item.ID, &item.ReporterHandle, &item.ReportedHandle, &item.Reason, &item.Detail,
		&item.Status, &item.AssignedAdminUserID, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	return item, err
}

func (s *Service) appeal(ctx context.Context, appealID string) (Appeal, error) {
	var item Appeal
	err := s.db.QueryRowContext(ctx, `
		SELECT a.id,p.handle,a.message,a.status,COALESCE(a.assigned_admin_user_id,''),a.response_note,a.created_at,a.updated_at
		FROM moderation_appeals a JOIN profiles p ON p.user_id=a.user_id WHERE a.id=?
	`, strings.TrimSpace(appealID)).Scan(&item.ID, &item.Handle, &item.Message, &item.Status, &item.AssignedAdminUserID, &item.ResponseNote, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Appeal{}, ErrNotFound
	}
	return item, err
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireRole(ctx context.Context, q rowQuerier, userID string) (Role, error) {
	var role string
	err := q.QueryRowContext(ctx, `SELECT role FROM moderation_admins WHERE user_id=?`, strings.TrimSpace(userID)).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", err
	}
	return Role(role), nil
}

func reportTarget(ctx context.Context, q rowQuerier, reportID string) (string, error) {
	var userID string
	err := q.QueryRowContext(ctx, `SELECT reported_user_id FROM reports WHERE id=?`, strings.TrimSpace(reportID)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func resolveHandle(ctx context.Context, q rowQuerier, handle string) (string, error) {
	var userID string
	err := q.QueryRowContext(ctx, `SELECT user_id FROM profiles WHERE handle=?`, normalizeHandle(handle)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func insertAction(ctx context.Context, tx *sql.Tx, role Role, actorUserID, targetUserID, reportID, action, note, now string) error {
	_, err := insertActionID(ctx, tx, role, actorUserID, targetUserID, reportID, action, note, now)
	return err
}

func insertActionID(ctx context.Context, tx *sql.Tx, role Role, actorUserID, targetUserID, reportID, action, note, now string) (string, error) {
	id, err := randomID("mod_")
	if err != nil {
		return "", err
	}
	var target any
	if strings.TrimSpace(targetUserID) != "" {
		target = strings.TrimSpace(targetUserID)
	}
	var report any
	if strings.TrimSpace(reportID) != "" {
		report = strings.TrimSpace(reportID)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO moderation_actions(id,actor_user_id,actor_role,target_user_id,report_id,action,note,created_at)
		VALUES(?,?,?,?,?,?,?,?)
	`, id, strings.TrimSpace(actorUserID), string(role), target, report, action, note, now); err != nil {
		return "", fmt.Errorf("record moderation action: %w", err)
	}
	return id, nil
}

func scanAppeals(rows *sql.Rows) ([]Appeal, error) {
	items := make([]Appeal, 0)
	for rows.Next() {
		var item Appeal
		if err := rows.Scan(&item.ID, &item.Handle, &item.Message, &item.Status, &item.AssignedAdminUserID, &item.ResponseNote, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func normalizeHandle(handle string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
}

func randomID(prefix string) (string, error) {
	buffer := make([]byte, 18)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buffer), nil
}
