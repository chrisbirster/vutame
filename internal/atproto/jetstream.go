package atproto

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	jetstreamCursorName = "vutame-appview"
	maxJetstreamMessage = 2 << 20
	websocketGUID        = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

type JetstreamEvent struct {
	DID      string           `json:"did"`
	Kind     string           `json:"kind"`
	TimeUS   int64            `json:"time_us"`
	Cursor   int64            `json:"cursor"`
	Commit   *JetstreamCommit `json:"commit,omitempty"`
	Identity *struct {
		Handle string `json:"handle"`
	} `json:"identity,omitempty"`
	Account *struct {
		Active bool   `json:"active"`
		Status string `json:"status,omitempty"`
	} `json:"account,omitempty"`
}

type JetstreamCommit struct {
	Operation  string         `json:"operation"`
	Collection string         `json:"collection"`
	RKey       string         `json:"rkey"`
	CID        string         `json:"cid,omitempty"`
	Record     map[string]any `json:"record,omitempty"`
}

func (s *Store) JetstreamEnabled() bool {
	return strings.TrimSpace(s.config.JetstreamURL) != ""
}

func (s *Store) RunJetstream(ctx context.Context) {
	if !s.JetstreamEnabled() {
		return
	}
	backoff := time.Second
	for ctx.Err() == nil {
		if err := s.consumeJetstream(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("AT Protocol Jetstream disconnected", "error", err, "retry_in", backoff)
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}
}

func (s *Store) consumeJetstream(ctx context.Context) error {
	cursor, err := s.jetstreamCursor(ctx)
	if err != nil {
		return err
	}
	rwc, err := s.openJetstream(ctx, cursor)
	if err != nil {
		return err
	}
	defer rwc.Close()
	for {
		message, err := readWebSocketMessage(rwc, rwc)
		if err != nil {
			return err
		}
		if len(message) == 0 {
			continue
		}
		if err := s.ProcessJetstreamEvent(ctx, message); err != nil {
			// Malformed/irrelevant records must not poison a public stream. A JSON
			// envelope error is retriable because it can indicate framing damage;
			// record-validation errors are safely skipped while still advancing the
			// stream cursor only when an envelope supplied one.
			if !errors.Is(err, ErrInvalidIdentity) {
				return err
			}
		}
	}
}

func (s *Store) ProcessJetstreamEvent(ctx context.Context, payload []byte) error {
	if len(payload) == 0 || len(payload) > maxJetstreamMessage {
		return fmt.Errorf("invalid Jetstream event size")
	}
	var event JetstreamEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode Jetstream event: %w", err)
	}
	event.DID = strings.TrimSpace(event.DID)
	if !strings.HasPrefix(event.DID, "did:") {
		return ErrInvalidIdentity
	}

	var handlingErr error
	switch event.Kind {
	case "commit":
		if event.Commit == nil {
			handlingErr = ErrInvalidIdentity
			break
		}
		commit := event.Commit
		if commit.Collection != ProfileCollection && commit.Collection != LinkCollection {
			break
		}
		switch commit.Operation {
		case "create", "update":
			if commit.Record == nil || strings.TrimSpace(commit.RKey) == "" {
				handlingErr = ErrInvalidIdentity
				break
			}
			handlingErr = s.indexPortableRecord(ctx, event.DID, commit.Collection, commit.RKey, strings.TrimSpace(commit.CID), commit.Record, s.now().UTC())
		case "delete":
			handlingErr = s.deleteIndexedPortableRecord(ctx, event.DID, commit.Collection, commit.RKey)
		}
	case "account":
		if event.Account != nil && (!event.Account.Active || event.Account.Status == "takendown" || event.Account.Status == "deactivated") {
			handlingErr = s.purgeIndexedDID(ctx, event.DID)
		}
	case "identity":
		// Identity events invalidate mutable handles. Resolve the DID again so a
		// stale portable record cannot claim a verified AT handle. Resolution
		// failures leave the portable record available by DID.
		if identity, err := s.ResolveIdentity(ctx, event.DID); err == nil && identity.Handle != "" {
			_, handlingErr = s.db.ExecContext(ctx, `UPDATE atproto_accounts SET handle=?,pds_url=?,updated_at=? WHERE did=?`, identity.Handle, identity.PDSURL, s.now().UTC().Format(time.RFC3339Nano), event.DID)
		}
	}
	if handlingErr != nil && !errors.Is(handlingErr, ErrInvalidIdentity) {
		return handlingErr
	}
	cursor := event.Cursor
	if cursor <= 0 {
		cursor = event.TimeUS
	}
	if cursor > 0 {
		if err := s.saveJetstreamCursor(ctx, cursor); err != nil {
			return err
		}
	}
	return handlingErr
}

func (s *Store) jetstreamCursor(ctx context.Context) (int64, error) {
	var cursor int64
	err := s.db.QueryRowContext(ctx, `SELECT cursor_us FROM atproto_jetstream_state WHERE name=?`, jetstreamCursorName).Scan(&cursor)
	if errors.Is(err, sqlErrNoRowsSentinel) {
		return 0, nil
	}
	return cursor, err
}

func (s *Store) saveJetstreamCursor(ctx context.Context, cursor int64) error {
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO atproto_jetstream_state(name,cursor_us,updated_at) VALUES(?,?,?)
		ON CONFLICT(name) DO UPDATE SET cursor_us=CASE WHEN excluded.cursor_us > cursor_us THEN excluded.cursor_us ELSE cursor_us END,updated_at=excluded.updated_at
	`, jetstreamCursorName, cursor, now)
	return err
}

func (s *Store) openJetstream(ctx context.Context, cursor int64) (io.ReadWriteCloser, error) {
	raw := strings.TrimSpace(s.config.JetstreamURL)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, ErrInvalidIdentity
	}
	if parsed.Scheme != "wss" {
		if !(s.config.AllowHTTP && parsed.Scheme == "ws" && developmentHost(parsed.Hostname())) {
			return nil, ErrInvalidIdentity
		}
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/subscribe"
	}
	query := parsed.Query()
	query.Add("wantedCollections", ProfileCollection)
	query.Add("wantedCollections", LinkCollection)
	if cursor > 0 {
		query.Set("cursor", strconv.FormatInt(cursor, 10))
	}
	parsed.RawQuery = query.Encode()

	httpURL := *parsed
	if parsed.Scheme == "wss" {
		httpURL.Scheme = "https"
	} else {
		httpURL.Scheme = "http"
	}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpURL.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", key)

	response, err := jetstreamHTTPClient(s.config.AllowHTTP).Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
		return nil, fmt.Errorf("Jetstream websocket upgrade returned HTTP %d", response.StatusCode)
	}
	expected := websocketAccept(key)
	if !strings.EqualFold(strings.TrimSpace(response.Header.Get("Upgrade")), "websocket") || strings.TrimSpace(response.Header.Get("Sec-WebSocket-Accept")) != expected {
		response.Body.Close()
		return nil, fmt.Errorf("invalid Jetstream websocket upgrade")
	}
	rwc, ok := response.Body.(io.ReadWriteCloser)
	if !ok {
		response.Body.Close()
		return nil, fmt.Errorf("Jetstream transport does not expose upgraded connection")
	}
	return rwc, nil
}

func jetstreamHTTPClient(allowHTTP bool) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if allowHTTP && developmentHost(host) {
				return dialer.DialContext(ctx, network, address)
			}
			addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(addresses) == 0 {
				return nil, fmt.Errorf("resolve Jetstream host")
			}
			for _, candidate := range addresses {
				if publicATAddress(candidate) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
				}
			}
			return nil, fmt.Errorf("Jetstream host does not resolve to a public address")
		},
	}
	return &http.Client{Transport: transport}
}

func websocketAccept(key string) string {
	digest := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(digest[:])
}

func readWebSocketMessage(r io.Reader, w io.Writer) ([]byte, error) {
	message := make([]byte, 0, 4096)
	started := false
	for {
		var header [2]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return nil, err
		}
		fin := header[0]&0x80 != 0
		opcode := header[0] & 0x0f
		masked := header[1]&0x80 != 0
		if masked {
			return nil, fmt.Errorf("Jetstream server sent masked websocket frame")
		}
		length := int64(header[1] & 0x7f)
		switch length {
		case 126:
			var size [2]byte
			if _, err := io.ReadFull(r, size[:]); err != nil { return nil, err }
			length = int64(binary.BigEndian.Uint16(size[:]))
		case 127:
			var size [8]byte
			if _, err := io.ReadFull(r, size[:]); err != nil { return nil, err }
			value := binary.BigEndian.Uint64(size[:])
			if value > maxJetstreamMessage { return nil, fmt.Errorf("Jetstream websocket message too large") }
			length = int64(value)
		}
		if length < 0 || length > maxJetstreamMessage || int64(len(message))+length > maxJetstreamMessage {
			return nil, fmt.Errorf("Jetstream websocket message too large")
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(r, payload); err != nil { return nil, err }
		switch opcode {
		case 0x8:
			_ = writeWebSocketControl(w, 0x8, payload)
			return nil, io.EOF
		case 0x9:
			if err := writeWebSocketControl(w, 0xA, payload); err != nil { return nil, err }
			continue
		case 0xA:
			continue
		case 0x1:
			if started { return nil, fmt.Errorf("unexpected new Jetstream text frame") }
			started = true
			message = append(message, payload...)
		case 0x0:
			if !started { return nil, fmt.Errorf("unexpected Jetstream continuation frame") }
			message = append(message, payload...)
		default:
			return nil, fmt.Errorf("unsupported Jetstream websocket opcode %d", opcode)
		}
		if fin && started {
			return message, nil
		}
	}
}

func writeWebSocketControl(w io.Writer, opcode byte, payload []byte) error {
	if len(payload) > 125 {
		payload = payload[:125]
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil { return err }
	frame := make([]byte, 0, 2+4+len(payload))
	frame = append(frame, 0x80|opcode, 0x80|byte(len(payload)))
	frame = append(frame, mask...)
	for index, value := range payload {
		frame = append(frame, value^mask[index%4])
	}
	_, err := w.Write(frame)
	return err
}

// database/sql's sentinel is surfaced through this package-level alias so the
// cursor path remains easy to exercise without exporting database internals.
var sqlErrNoRowsSentinel = errors.New("sql: no rows in result set")

var _ netip.Addr
