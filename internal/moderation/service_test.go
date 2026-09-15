package moderation

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	_ "modernc.org/sqlite"
)

func TestAdminAuthorizationAndReportWorkflow(t *testing.T) {
	db := newModerationTestDB(t)
	seedModerationFixtures(t, db)
	service, err := NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	service.SetNow(func() time.Time { return now })

	if _, err := service.Queue(context.Background(), "usr_reporter", "", 50); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin queue error=%v", err)
	}
	queue, err := service.Queue(context.Background(), "usr_admin", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].ID != "rpt_1" || queue[0].Status != "open" {
		t.Fatalf("queue=%+v", queue)
	}

	assigned, err := service.Assign(context.Background(), "usr_admin", "rpt_1")
	if err != nil {
		t.Fatal(err)
	}
	if assigned.Status != "reviewing" || assigned.AssignedAdminUserID != "usr_admin" {
		t.Fatalf("assigned=%+v", assigned)
	}
	if err := service.AddNote(context.Background(), "usr_admin", "rpt_1", "reviewed supporting context"); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve(context.Background(), "usr_admin", "rpt_1", "resolved", "policy action complete")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != "resolved" {
		t.Fatalf("resolved=%+v", resolved)
	}

	actions, err := service.Actions(context.Background(), "usr_admin", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 3 {
		t.Fatalf("actions=%+v", actions)
	}
	seen := map[string]int{}
	for _, action := range actions {
		seen[action.Action]++
	}
	for _, want := range []string{"assign", "note", "resolve"} {
		if seen[want] != 1 {
			t.Fatalf("missing or duplicate %q audit action: %+v", want, actions)
		}
	}
}

func TestTakedownAndRestorePublicPolicy(t *testing.T) {
	db := newModerationTestDB(t)
	seedModerationFixtures(t, db)
	service, err := NewService(db)
	if err != nil {
		t.Fatal(err)
	}

	policy, err := service.Policy(context.Background(), "target")
	if err != nil {
		t.Fatal(err)
	}
	if policy.Hidden() || !policy.Discoverable() {
		t.Fatalf("initial policy=%+v", policy)
	}

	if err := service.SetState(context.Background(), "usr_admin", "target", "takedown", "credible impersonation report"); err != nil {
		t.Fatal(err)
	}
	policy, err = service.Policy(context.Background(), "target")
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Hidden() || !policy.TakenDown || policy.State != "suspended" {
		t.Fatalf("takedown policy=%+v", policy)
	}
	localDID, err := service.PolicyForDID(context.Background(), "did:plc:target")
	if err != nil {
		t.Fatal(err)
	}
	if !localDID.Hidden() {
		t.Fatalf("local DID policy=%+v", localDID)
	}
	remoteDID, err := service.PolicyForDID(context.Background(), "did:plc:remote")
	if err != nil {
		t.Fatal(err)
	}
	if remoteDID.Local || remoteDID.Hidden() {
		t.Fatalf("remote DID policy=%+v", remoteDID)
	}

	if err := service.SetState(context.Background(), "usr_admin", "target", "active", "appeal accepted"); err != nil {
		t.Fatal(err)
	}
	policy, err = service.Policy(context.Background(), "target")
	if err != nil {
		t.Fatal(err)
	}
	if policy.Hidden() || policy.TakenDown || policy.State != "active" {
		t.Fatalf("restored policy=%+v", policy)
	}
}

func TestCreatorAppealAndAbuseStats(t *testing.T) {
	db := newModerationTestDB(t)
	seedModerationFixtures(t, db)
	service, err := NewService(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	service.SetNow(func() time.Time { return now })

	appeal, err := service.CreateAppeal(context.Background(), "usr_target", "Please review the restriction.")
	if err != nil {
		t.Fatal(err)
	}
	if appeal.Status != "open" || appeal.Handle != "target" {
		t.Fatalf("appeal=%+v", appeal)
	}
	queue, err := service.AppealQueue(context.Background(), "usr_admin", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].ID != appeal.ID {
		t.Fatalf("appeal queue=%+v", queue)
	}
	updated, err := service.ReviewAppeal(context.Background(), "usr_admin", appeal.ID, "resolved", "restored after review")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "resolved" || updated.ResponseNote == "" {
		t.Fatalf("updated appeal=%+v", updated)
	}

	stats, err := service.Stats(context.Background(), "usr_admin", now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Reports != 1 || stats.OpenReports != 1 || stats.ReportsByReason["impersonation"] != 1 {
		t.Fatalf("stats=%+v", stats)
	}
}

func newModerationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "moderation.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedModerationFixtures(t *testing.T, db *sql.DB) {
	t.Helper()
	now := "2026-09-15T07:00:00Z"
	for _, item := range []struct{ id, email string }{
		{"usr_admin", "admin@example.com"},
		{"usr_reporter", "reporter@example.com"},
		{"usr_target", "target@example.com"},
	} {
		if _, err := db.Exec(`INSERT INTO users(id,email,email_verified,created_at,updated_at) VALUES(?,?,1,?,?)`, item.id, item.email, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO profiles(user_id,handle,display_name,bio,avatar_url,theme,verified,atproto_did,created_at,updated_at) VALUES('usr_reporter','reporter','Reporter','','','midnight',0,NULL,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles(user_id,handle,display_name,bio,avatar_url,theme,verified,atproto_did,created_at,updated_at) VALUES('usr_target','target','Target','','','midnight',0,'did:plc:target',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO moderation_admins(user_id,role,created_at,updated_at) VALUES('usr_admin','admin',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO reports(id,reporter_user_id,reported_user_id,reason,detail,status,created_at,updated_at) VALUES('rpt_1','usr_reporter','usr_target','impersonation','looks copied','open',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
}
