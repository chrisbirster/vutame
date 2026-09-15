package atproto

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestProcessJetstreamEventIndexesReplaysDeletesAndAdvancesCursor(t *testing.T) {
	db := newATProtoTestDB(t)
	store, err := NewStore(db, []byte("0123456789abcdef0123456789abcdef"), Config{ClientID:"https://vutame.example/oauth-client-metadata.json",RedirectURI:"https://vutame.example/callback"})
	if err != nil { t.Fatal(err) }
	store.SetNow(func() time.Time { return time.Date(2026,9,15,2,0,0,0,time.UTC) })

	profileEvent := map[string]any{
		"did":"did:plc:portable","kind":"commit","cursor":101,
		"commit":map[string]any{"operation":"create","collection":ProfileCollection,"rkey":"self","cid":"cid-profile","record":map[string]any{
			"$type":ProfileCollection,"displayName":"Portable Creator","handle":"portable-vuta","bio":"from my PDS","theme":"paper","verified":true,"updatedAt":"2026-09-15T02:00:00Z",
		}},
	}
	if err := processTestJetstream(t, store, profileEvent); err != nil { t.Fatalf("profile event: %v", err) }
	// At-least-once replay is idempotent.
	if err := processTestJetstream(t, store, profileEvent); err != nil { t.Fatalf("profile replay: %v", err) }

	linkEvent := map[string]any{
		"did":"did:plc:portable","kind":"commit","cursor":102,
		"commit":map[string]any{"operation":"update","collection":LinkCollection,"rkey":"link-one","cid":"cid-link","record":map[string]any{
			"$type":LinkCollection,"label":"Project","url":"https://example.com/project","kind":"project","featured":true,"position":0,"updatedAt":"2026-09-15T02:00:01Z",
		}},
	}
	if err := processTestJetstream(t, store, linkEvent); err != nil { t.Fatalf("link event: %v", err) }
	item, err := store.IndexedProfile(context.Background(), "did:plc:portable")
	if err != nil { t.Fatal(err) }
	if item.DisplayName != "Portable Creator" || item.Verified || len(item.Links) != 1 || item.Links[0].Label != "Project" {
		t.Fatalf("unexpected indexed profile %+v", item)
	}
	// Self-asserted portable `verified:true` is deliberately ignored.
	if item.Verified { t.Fatal("portable record must not self-assert Vutame verification") }

	deleteEvent := map[string]any{"did":"did:plc:portable","kind":"commit","cursor":103,"commit":map[string]any{"operation":"delete","collection":LinkCollection,"rkey":"link-one"}}
	if err := processTestJetstream(t, store, deleteEvent); err != nil { t.Fatalf("delete event: %v", err) }
	item, err = store.IndexedProfile(context.Background(), "did:plc:portable")
	if err != nil { t.Fatal(err) }
	if len(item.Links) != 0 { t.Fatalf("deleted link still indexed: %+v", item.Links) }
	cursor, err := store.jetstreamCursor(context.Background())
	if err != nil || cursor != 103 { t.Fatalf("cursor=%d err=%v want 103", cursor, err) }

	// Older replay cannot move the persisted cursor backwards.
	profileEvent["cursor"] = 100
	if err := processTestJetstream(t, store, profileEvent); err != nil { t.Fatal(err) }
	cursor, _ = store.jetstreamCursor(context.Background())
	if cursor != 103 { t.Fatalf("cursor regressed to %d", cursor) }
}

func TestProcessJetstreamEventPurgesInactiveAccountAndSkipsMalformedRecord(t *testing.T) {
	db := newATProtoTestDB(t)
	store, err := NewStore(db, []byte("abcdef0123456789abcdef0123456789"), Config{ClientID:"https://vutame.example/oauth-client-metadata.json",RedirectURI:"https://vutame.example/callback"})
	if err != nil { t.Fatal(err) }
	valid := map[string]any{"did":"did:plc:gone","kind":"commit","cursor":20,"commit":map[string]any{"operation":"create","collection":ProfileCollection,"rkey":"self","cid":"c1","record":map[string]any{"$type":ProfileCollection,"displayName":"Gone","handle":"gone-vuta","theme":"midnight","updatedAt":"2026-09-15T02:00:00Z"}}}
	if err := processTestJetstream(t, store, valid); err != nil { t.Fatal(err) }

	malformed := map[string]any{"did":"did:plc:gone","kind":"commit","cursor":21,"commit":map[string]any{"operation":"update","collection":LinkCollection,"rkey":"bad","cid":"c2","record":map[string]any{"$type":LinkCollection,"label":"Bad","url":"javascript:alert(1)","kind":"website","position":0,"updatedAt":"2026-09-15T02:00:00Z"}}}
	if err := processTestJetstream(t, store, malformed); !errors.Is(err, ErrInvalidIdentity) { t.Fatalf("malformed err=%v want ErrInvalidIdentity", err) }
	cursor, _ := store.jetstreamCursor(context.Background())
	if cursor != 21 { t.Fatalf("malformed record should advance envelope cursor, got %d", cursor) }

	inactive := map[string]any{"did":"did:plc:gone","kind":"account","cursor":22,"account":map[string]any{"active":false,"status":"deactivated"}}
	if err := processTestJetstream(t, store, inactive); err != nil { t.Fatal(err) }
	if _, err := store.IndexedProfile(context.Background(), "did:plc:gone"); !errors.Is(err, ErrIndexedNotFound) { t.Fatalf("inactive DID still indexed: %v", err) }
}

func TestReadWebSocketMessageHandlesPingAndFragmentedText(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(serverFrame(false, 0x1, []byte(`{"did":"did:plc:x",`)))
	stream.Write(serverFrame(true, 0x9, []byte("hi")))
	stream.Write(serverFrame(true, 0x0, []byte(`"kind":"account"}`)))
	var controls bytes.Buffer
	message, err := readWebSocketMessage(&stream, &controls)
	if err != nil { t.Fatal(err) }
	if string(message) != `{"did":"did:plc:x","kind":"account"}` { t.Fatalf("message=%q", message) }
	if controls.Len() < 6 || controls.Bytes()[0]&0x0f != 0xA || controls.Bytes()[1]&0x80 == 0 { t.Fatalf("client pong was not masked: %x", controls.Bytes()) }
}

func processTestJetstream(t *testing.T, store *Store, event map[string]any) error {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil { t.Fatal(err) }
	return store.ProcessJetstreamEvent(context.Background(), payload)
}

func serverFrame(fin bool, opcode byte, payload []byte) []byte {
	first := opcode
	if fin { first |= 0x80 }
	frame := []byte{first}
	if len(payload) < 126 {
		frame = append(frame, byte(len(payload)))
	} else {
		frame = append(frame, 126, 0, 0)
		binary.BigEndian.PutUint16(frame[len(frame)-2:], uint16(len(payload)))
	}
	return append(frame, payload...)
}
