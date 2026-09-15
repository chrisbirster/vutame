package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
	"github.com/chrisbirster/vutame/internal/moderation"
	_ "turso.tech/database/tursogo"
)

func TestModerationRunsOnTursoEngine(t *testing.T) {
	db, err := sql.Open("turso", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		t.Fatalf("apply Vutame schema on Turso engine: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for _, user := range []struct{ id, email string }{
		{"usr_admin", "admin-turso@example.com"},
		{"usr_reporter", "reporter-turso@example.com"},
		{"usr_target", "target-turso@example.com"},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO users(id,email,email_verified,created_at,updated_at) VALUES(?,?,1,?,?)`, user.id, user.email, now, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO profiles(user_id,handle,display_name,bio,avatar_url,theme,verified,atproto_did,created_at,updated_at) VALUES('usr_reporter','reporter-turso','Reporter','','','midnight',0,NULL,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO profiles(user_id,handle,display_name,bio,avatar_url,theme,verified,atproto_did,created_at,updated_at) VALUES('usr_target','target-turso','Target','','','midnight',0,'did:plc:target-turso',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO moderation_admins(user_id,role,created_at,updated_at) VALUES('usr_admin','admin',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO reports(id,reporter_user_id,reported_user_id,reason,detail,status,created_at,updated_at) VALUES('rpt_turso','usr_reporter','usr_target','spam','bulk links','open',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}

	service, err := moderation.NewService(db)
	if err != nil {
		t.Fatalf("open moderation service on Turso engine: %v", err)
	}
	queue, err := service.Queue(ctx, "usr_admin", "", 20)
	if err != nil {
		t.Fatalf("queue reports: %v", err)
	}
	if len(queue) != 1 || queue[0].ID != "rpt_turso" {
		t.Fatalf("queue=%+v", queue)
	}
	if _, err := service.Assign(ctx, "usr_admin", "rpt_turso"); err != nil {
		t.Fatalf("assign report: %v", err)
	}
	if err := service.SetState(ctx, "usr_admin", "target-turso", "takedown", "confirmed spam operation"); err != nil {
		t.Fatalf("takedown profile: %v", err)
	}
	policy, err := service.Policy(ctx, "target-turso")
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if !policy.Hidden() || !policy.TakenDown {
		t.Fatalf("policy=%+v", policy)
	}
	appeal, err := service.CreateAppeal(ctx, "usr_target", "I would like this reviewed.")
	if err != nil {
		t.Fatalf("create appeal: %v", err)
	}
	if appeal.Status != "open" {
		t.Fatalf("appeal=%+v", appeal)
	}
	actions, err := service.Actions(ctx, "usr_admin", 20)
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	seenTakedown := false
	seenAssign := false
	for _, action := range actions {
		seenTakedown = seenTakedown || action.Action == "takedown"
		seenAssign = seenAssign || action.Action == "assign"
	}
	if !seenTakedown || !seenAssign {
		t.Fatalf("actions=%+v", actions)
	}
}
