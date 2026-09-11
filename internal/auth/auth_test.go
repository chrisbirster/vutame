package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/chrisbirster/vutame/internal/dbschema"
)

type captureSender struct {
	email string
	code  string
	err   error
}

func (s *captureSender) SendCode(_ context.Context, email, code string) error {
	s.email = email
	s.code = code
	return s.err
}

func TestCodeSessionLifecycle(t *testing.T) {
	store := newAuthTestStore(t)
	sender := &captureSender{}
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	service, err := NewService(store, sender, []byte("0123456789abcdef0123456789abcdef"), Config{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}

	challengeID, err := service.RequestCode(context.Background(), "Chris@Example.COM")
	if err != nil {
		t.Fatal(err)
	}
	if sender.email != "chris@example.com" || len(sender.code) != 6 || challengeID == "" {
		t.Fatalf("delivery = %q %q challenge=%q", sender.email, sender.code, challengeID)
	}
	result, err := service.VerifyCode(context.Background(), challengeID, "chris@example.com", sender.code)
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.User.Email != "chris@example.com" || !result.User.EmailVerified {
		t.Fatalf("result = %#v", result)
	}
	user, err := service.Session(context.Background(), result.Token)
	if err != nil || user.ID != result.User.ID {
		t.Fatalf("session user=%#v err=%v", user, err)
	}
	if _, err := service.VerifyCode(context.Background(), challengeID, "chris@example.com", sender.code); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("reused code err=%v, want ErrInvalidCode", err)
	}
	if err := service.Logout(context.Background(), result.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Session(context.Background(), result.Token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("logged-out session err=%v", err)
	}
}

func TestWrongCodeCountsAttempts(t *testing.T) {
	store := newAuthTestStore(t)
	sender := &captureSender{}
	service, err := NewService(store, sender, []byte("0123456789abcdef0123456789abcdef"), Config{MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	id, err := service.RequestCode(context.Background(), "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := service.VerifyCode(context.Background(), id, "a@example.com", "000000"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("wrong code err=%v", err)
		}
	}
	if _, err := service.VerifyCode(context.Background(), id, "a@example.com", sender.code); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("locked challenge err=%v", err)
	}
}

func TestDeliveryFailureRemovesChallenge(t *testing.T) {
	store := newAuthTestStore(t)
	sender := &captureSender{err: ErrDeliveryUnavailable}
	service, err := NewService(store, sender, []byte("0123456789abcdef0123456789abcdef"), Config{})
	if err != nil {
		t.Fatal(err)
	}
	id, err := service.RequestCode(context.Background(), "a@example.com")
	if !errors.Is(err, ErrDeliveryUnavailable) || id != "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM auth_challenges`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("challenge count=%d, want 0", count)
	}
}

func newAuthTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.db")
	dsn := "file:" + path
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(dbschema.SQL); err != nil {
		db.Close()
		t.Fatalf("apply schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLite(dsn)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestNormalizeEmail(t *testing.T) {
	for _, email := range []string{"", "not-an-email", "Name <a@example.com>"} {
		if _, err := NormalizeEmail(email); !errors.Is(err, ErrInvalidEmail) {
			t.Fatalf("NormalizeEmail(%q) err=%v", email, err)
		}
	}
	got, err := NormalizeEmail(" A@Example.COM ")
	if err != nil || got != "a@example.com" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestStoreSchemaError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	store, err := OpenSQLite("file:" + path)
	if err == nil {
		store.Close()
		t.Fatal("expected missing-schema error")
	}
	if got := fmt.Sprint(err); got == "" {
		t.Fatal("expected descriptive error")
	}
}
