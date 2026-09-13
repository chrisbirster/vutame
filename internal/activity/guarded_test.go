package activity

import (
	"context"
	"testing"
	"time"
)

func TestGuardedActivityHidesMutedPrivateAndRestrictedCreators(t *testing.T) {
	db := openActivityDB(t)
	base, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewGuardedStore(base, db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	insertActivityCreator(t, db, "usr_alice", "alice")
	insertActivityCreator(t, db, "usr_bob", "bob")
	insertActivityCreator(t, db, "usr_cara", "cara")
	insertActivityCreator(t, db, "usr_dan", "dan")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO follows (follower_user_id, following_user_id, created_at) VALUES ('usr_alice','usr_bob',?), ('usr_alice','usr_cara',?), ('usr_alice','usr_dan',?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	if err := base.Record(ctx, "usr_bob", KindProfileUpdated, "", "Bob"); err != nil {
		t.Fatal(err)
	}
	if err := base.Record(ctx, "usr_cara", KindProfileUpdated, "", "Cara"); err != nil {
		t.Fatal(err)
	}
	if err := base.Record(ctx, "usr_dan", KindProfileUpdated, "", "Dan"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mutes (muter_user_id, muted_user_id, created_at) VALUES ('usr_alice','usr_bob',?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO creator_privacy (user_id, discoverable, activity_visible, allow_follows, updated_at) VALUES ('usr_cara',1,0,1,?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO moderation_profiles (user_id,state,note,updated_at) VALUES ('usr_dan','restricted','test',?)`, now); err != nil {
		t.Fatal(err)
	}

	feed, err := store.Following(ctx, "usr_alice", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Events) != 0 {
		t.Fatalf("guarded following feed leaked hidden events: %#v", feed)
	}
	recent, err := store.RecentForViewer(ctx, "usr_alice", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Events) != 0 {
		t.Fatalf("guarded recent feed leaked hidden events: %#v", recent)
	}

	if _, err := db.Exec(`DELETE FROM mutes; DELETE FROM creator_privacy; DELETE FROM moderation_profiles`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO blocks (blocker_user_id, blocked_user_id, created_at) VALUES ('usr_bob','usr_alice',?)`, now); err != nil {
		t.Fatal(err)
	}
	recent, err = store.RecentForViewer(ctx, "usr_alice", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range recent.Events {
		if event.Handle == "bob" {
			t.Fatal("blocked author leaked into recent feed")
		}
	}
}
