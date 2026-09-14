package operations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func (s *Store) CreateWebhook(ctx context.Context, userID, rawURL string, events []string) (CreatedWebhook, error) {
	if err := s.requireProfile(ctx, strings.TrimSpace(userID)); err != nil {
		return CreatedWebhook{}, err
	}
	webhookURL, err := ValidateWebhookURL(rawURL)
	if err != nil {
		return CreatedWebhook{}, err
	}
	normalizedEvents, err := NormalizeWebhookEvents(events)
	if err != nil {
		return CreatedWebhook{}, err
	}
	id, err := randomID("wh_", 12)
	if err != nil {
		return CreatedWebhook{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO webhooks (id, user_id, url, events, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)
	`, id, userID, webhookURL, strings.Join(normalizedEvents, ","), now, now)
	if err != nil {
		return CreatedWebhook{}, fmt.Errorf("create webhook: %w", err)
	}
	item := Webhook{ID: id, URL: webhookURL, Events: normalizedEvents, Active: true, CreatedAt: now, UpdatedAt: now}
	return CreatedWebhook{Webhook: item, SigningSecret: base64.RawURLEncoding.EncodeToString(s.webhookSecret(id))}, nil
}

func (s *Store) Webhooks(ctx context.Context, userID string) ([]Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, url, events, active, created_at, updated_at FROM webhooks
		WHERE user_id = ? ORDER BY created_at DESC, id DESC
	`, strings.TrimSpace(userID))
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer rows.Close()
	items := make([]Webhook, 0)
	for rows.Next() {
		var item Webhook
		var events string
		var active int
		if err := rows.Scan(&item.ID, &item.URL, &events, &active, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Events = splitCSV(events)
		item.Active = active == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DeleteWebhook(ctx context.Context, userID, webhookID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ? AND user_id = ?`, strings.TrimSpace(webhookID), strings.TrimSpace(userID))
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrInvalidWebhook
	}
	return nil
}

func (s *Store) QueueWebhookEvent(ctx context.Context, userID, event, payload string) error {
	if _, ok := supportedWebhookEvents[event]; !ok {
		return fmt.Errorf("%w: unsupported event %q", ErrInvalidWebhook, event)
	}
	if len(payload) > 128*1024 {
		return fmt.Errorf("%w: payload too large", ErrInvalidWebhook)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, events FROM webhooks WHERE user_id = ? AND active = 1`, strings.TrimSpace(userID))
	if err != nil {
		return fmt.Errorf("find webhooks for event: %w", err)
	}
	type target struct{ id, events string }
	targets := make([]target, 0)
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.id, &item.events); err != nil {
			_ = rows.Close()
			return err
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	now := s.now().UTC().Format(time.RFC3339Nano)
	for _, item := range targets {
		if !containsScope(splitCSV(item.events), event) {
			continue
		}
		id, err := randomID("del_", 12)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO webhook_deliveries (id, webhook_id, user_id, event, payload, status, attempts, next_attempt_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'pending', 0, ?, ?, ?)
		`, id, item.id, userID, event, payload, now, now, now); err != nil {
			return fmt.Errorf("queue webhook delivery: %w", err)
		}
	}
	return nil
}

func (s *Store) QueueWebhookTest(ctx context.Context, userID, webhookID string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM webhooks WHERE id = ? AND user_id = ? AND active = 1`, webhookID, userID).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return ErrInvalidWebhook
	}
	id, err := randomID("del_", 12)
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	payload := `{"event":"webhook.test","source":"vutame"}`
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO webhook_deliveries (id, webhook_id, user_id, event, payload, status, attempts, next_attempt_at, created_at, updated_at)
		VALUES (?, ?, ?, 'webhook.test', ?, 'pending', 0, ?, ?, ?)
	`, id, webhookID, userID, payload, now, now, now)
	return err
}

type pendingDelivery struct {
	ID        string
	WebhookID string
	URL       string
	Event     string
	Payload   string
	Attempts  int
}

func (s *Store) DispatchPending(ctx context.Context, limit int) error {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.webhook_id, w.url, d.event, d.payload, d.attempts
		FROM webhook_deliveries d JOIN webhooks w ON w.id = d.webhook_id
		WHERE d.status = 'pending' AND w.active = 1 AND d.next_attempt_at <= ?
		ORDER BY d.created_at LIMIT ?
	`, s.now().UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return fmt.Errorf("load pending webhooks: %w", err)
	}
	items := make([]pendingDelivery, 0)
	for rows.Next() {
		var item pendingDelivery
		if err := rows.Scan(&item.ID, &item.WebhookID, &item.URL, &item.Event, &item.Payload, &item.Attempts); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	client := safeWebhookClient()
	for _, item := range items {
		if err := s.deliver(ctx, client, item); err != nil {
			continue
		}
	}
	return nil
}

func (s *Store) deliver(ctx context.Context, client *http.Client, item pendingDelivery) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, item.URL, bytes.NewBufferString(item.Payload))
	if err != nil {
		s.markDeliveryFailure(ctx, item, 0, err)
		return err
	}
	mac := hmac.New(sha256.New, s.webhookSecret(item.WebhookID))
	_, _ = mac.Write([]byte(item.Payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Vutame-Webhooks/1.0")
	request.Header.Set("X-Vutame-Event", item.Event)
	request.Header.Set("X-Vutame-Delivery", item.ID)
	request.Header.Set("X-Vutame-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response, err := client.Do(request)
	if err != nil {
		s.markDeliveryFailure(ctx, item, 0, err)
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32*1024))
	_ = response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err := fmt.Errorf("webhook response status %d", response.StatusCode)
		s.markDeliveryFailure(ctx, item, response.StatusCode, err)
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		UPDATE webhook_deliveries SET status='delivered', attempts=attempts+1, response_code=?, delivered_at=?, last_error='', updated_at=? WHERE id=?
	`, response.StatusCode, now, now, item.ID)
	return err
}

func (s *Store) markDeliveryFailure(ctx context.Context, item pendingDelivery, code int, cause error) {
	attempts := item.Attempts + 1
	status := "pending"
	if attempts >= 8 {
		status = "failed"
	}
	delay := time.Minute << minInt(attempts-1, 6)
	next := s.now().UTC().Add(delay).Format(time.RFC3339Nano)
	message := strings.TrimSpace(cause.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	var responseCode any
	if code > 0 {
		responseCode = code
	}
	_, _ = s.db.ExecContext(ctx, `
		UPDATE webhook_deliveries SET status=?, attempts=?, response_code=?, next_attempt_at=?, last_error=?, updated_at=? WHERE id=?
	`, status, attempts, responseCode, next, message, s.now().UTC().Format(time.RFC3339Nano), item.ID)
}

func (s *Store) RunWebhookWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			workerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_ = s.DispatchPending(workerCtx, 20)
			cancel()
		}
	}
}

func safeWebhookClient() *http.Client {
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   4 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(addresses) == 0 {
				return nil, fmt.Errorf("resolve webhook host")
			}
			for _, candidate := range addresses {
				if !publicAddress(candidate) {
					continue
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
			}
			return nil, fmt.Errorf("webhook host does not resolve to a public address")
		},
	}
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many webhook redirects")
		}
		if _, err := ValidateWebhookURL(req.URL.String()); err != nil {
			return err
		}
		return nil
	}
	return client
}

func (s *Store) DeliveryCount(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM webhook_deliveries WHERE user_id = ?`, userID).Scan(&count)
	return count, err
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
