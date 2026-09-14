package operations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

func (s *Store) AddDomain(ctx context.Context, userID, hostname string) (Domain, error) {
	userID = strings.TrimSpace(userID)
	if err := s.requireProfile(ctx, userID); err != nil {
		return Domain{}, err
	}
	host, err := NormalizeHostname(hostname)
	if err != nil {
		return Domain{}, err
	}
	id, err := randomID("dom_", 12)
	if err != nil {
		return Domain{}, fmt.Errorf("generate domain id: %w", err)
	}
	token, err := randomID("verify_", 24)
	if err != nil {
		return Domain{}, fmt.Errorf("generate domain verification token: %w", err)
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO custom_domains (id, user_id, hostname, verification_token, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, userID, host, token, now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Domain{}, ErrDomainTaken
		}
		return Domain{}, fmt.Errorf("create custom domain: %w", err)
	}
	return decorateDomain(Domain{ID: id, Hostname: host, VerificationToken: token}), nil
}

func (s *Store) Domains(ctx context.Context, userID string) ([]Domain, error) {
	if err := s.requireProfile(ctx, strings.TrimSpace(userID)); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, hostname, verification_token, COALESCE(verified_at, '')
		FROM custom_domains WHERE user_id = ? ORDER BY created_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list custom domains: %w", err)
	}
	defer rows.Close()
	items := make([]Domain, 0)
	for rows.Next() {
		var item Domain
		if err := rows.Scan(&item.ID, &item.Hostname, &item.VerificationToken, &item.VerifiedAt); err != nil {
			return nil, fmt.Errorf("scan custom domain: %w", err)
		}
		items = append(items, decorateDomain(item))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom domains: %w", err)
	}
	return items, nil
}

func (s *Store) VerifyDomain(ctx context.Context, userID, domainID string) (Domain, error) {
	var item Domain
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, hostname, verification_token, COALESCE(verified_at, '')
		FROM custom_domains WHERE id = ? AND user_id = ?
	`, strings.TrimSpace(domainID), strings.TrimSpace(userID)).Scan(&item.ID, &item.Hostname, &item.VerificationToken, &item.VerifiedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Domain{}, ErrDomainNotFound
		}
		return Domain{}, fmt.Errorf("load custom domain: %w", err)
	}
	if item.VerifiedAt != "" {
		return decorateDomain(item), nil
	}
	txt, err := s.resolver.LookupTXT(ctx, "_vutame."+item.Hostname)
	if err != nil {
		return Domain{}, ErrVerificationPending
	}
	expected := "vutame-verification=" + item.VerificationToken
	matched := false
	for _, value := range txt {
		if strings.TrimSpace(value) == expected {
			matched = true
			break
		}
	}
	if !matched {
		return Domain{}, ErrVerificationPending
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Domain{}, fmt.Errorf("begin custom domain verification: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE custom_domains SET verified_at = ?, updated_at = ? WHERE id = ? AND user_id = ?`, now, now, item.ID, userID); err != nil {
		return Domain{}, fmt.Errorf("verify custom domain: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE profiles SET verified = 1, updated_at = ? WHERE user_id = ?`, now, userID); err != nil {
		return Domain{}, fmt.Errorf("mark creator verified: %w", err)
	}
	requestID, err := randomID("ver_", 12)
	if err != nil {
		return Domain{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO verification_requests (id, user_id, method, evidence, status, note, created_at, updated_at)
		VALUES (?, ?, 'custom_domain', ?, 'approved', 'DNS TXT proof verified automatically.', ?, ?)
	`, requestID, userID, item.Hostname, now, now); err != nil {
		return Domain{}, fmt.Errorf("record creator verification: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Domain{}, fmt.Errorf("commit custom domain verification: %w", err)
	}
	item.VerifiedAt = now
	return decorateDomain(item), nil
}

func (s *Store) DeleteDomain(ctx context.Context, userID, domainID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM custom_domains WHERE id = ? AND user_id = ?`, strings.TrimSpace(domainID), strings.TrimSpace(userID))
	if err != nil {
		return fmt.Errorf("delete custom domain: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrDomainNotFound
	}
	var proofs int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_domains WHERE user_id = ? AND verified_at IS NOT NULL`, userID).Scan(&proofs); err == nil && proofs == 0 {
		_, _ = s.db.ExecContext(ctx, `UPDATE profiles SET verified = 0, updated_at = ? WHERE user_id = ? AND atproto_did IS NULL`, s.now().UTC().Format(time.RFC3339Nano), userID)
	}
	return nil
}

func (s *Store) HandleForHost(ctx context.Context, host string) (string, error) {
	hostname := strings.TrimSpace(strings.ToLower(host))
	if splitHost, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = strings.Trim(splitHost, "[]")
	}
	hostname = strings.TrimSuffix(hostname, ".")
	var handle string
	if err := s.db.QueryRowContext(ctx, `
		SELECT p.handle FROM custom_domains d JOIN profiles p ON p.user_id = d.user_id
		WHERE d.hostname = ? COLLATE NOCASE AND d.verified_at IS NOT NULL
	`, hostname).Scan(&handle); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrDomainNotFound
		}
		return "", fmt.Errorf("resolve custom domain: %w", err)
	}
	return handle, nil
}

func (s *Store) CanonicalDomain(ctx context.Context, userID string) (string, error) {
	var hostname string
	if err := s.db.QueryRowContext(ctx, `
		SELECT hostname FROM custom_domains WHERE user_id = ? AND verified_at IS NOT NULL
		ORDER BY verified_at ASC, id ASC LIMIT 1
	`, strings.TrimSpace(userID)).Scan(&hostname); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrDomainNotFound
		}
		return "", err
	}
	return hostname, nil
}

func (s *Store) VerificationRequests(ctx context.Context, userID string) ([]VerificationRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, method, evidence, status, note, created_at, updated_at
		FROM verification_requests WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT 50
	`, strings.TrimSpace(userID))
	if err != nil {
		return nil, fmt.Errorf("list verification requests: %w", err)
	}
	defer rows.Close()
	items := make([]VerificationRequest, 0)
	for rows.Next() {
		var item VerificationRequest
		if err := rows.Scan(&item.ID, &item.Method, &item.Evidence, &item.Status, &item.Note, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func decorateDomain(item Domain) Domain {
	item.DNSName = "_vutame." + item.Hostname
	item.DNSValue = "vutame-verification=" + item.VerificationToken
	return item
}

func (s *Store) requireProfile(ctx context.Context, userID string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles WHERE user_id = ?`, userID).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return ErrProfileRequired
	}
	return nil
}
