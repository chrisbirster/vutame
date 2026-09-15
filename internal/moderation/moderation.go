package moderation

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrForbidden       = errors.New("moderation admin access required")
	ErrNotFound        = errors.New("moderation record not found")
	ErrProfileRequired = errors.New("profile required")
	ErrInvalidInput    = errors.New("invalid moderation input")
)

type Role string

const (
	RoleModerator Role = "moderator"
	RoleAdmin     Role = "admin"
)

type Report struct {
	ID                    string `json:"id"`
	ReporterHandle        string `json:"reporter_handle"`
	ReportedHandle        string `json:"reported_handle"`
	Reason                string `json:"reason"`
	Detail                string `json:"detail,omitempty"`
	Status                string `json:"status"`
	AssignedAdminUserID   string `json:"assigned_admin_user_id,omitempty"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}

type Action struct {
	ID           string `json:"id"`
	ActorUserID  string `json:"actor_user_id,omitempty"`
	ActorRole    string `json:"actor_role"`
	TargetHandle string `json:"target_handle,omitempty"`
	ReportID     string `json:"report_id,omitempty"`
	Action       string `json:"action"`
	Note         string `json:"note,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type Appeal struct {
	ID                  string `json:"id"`
	Handle              string `json:"handle"`
	Message             string `json:"message"`
	Status              string `json:"status"`
	AssignedAdminUserID string `json:"assigned_admin_user_id,omitempty"`
	ResponseNote        string `json:"response_note,omitempty"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type PublicPolicy struct {
	Local      bool   `json:"local"`
	State      string `json:"state"`
	TakenDown  bool   `json:"taken_down"`
}

func (p PublicPolicy) Hidden() bool {
	return p.Local && (p.TakenDown || p.State == "suspended")
}

func (p PublicPolicy) Discoverable() bool {
	return !p.Local || (!p.TakenDown && p.State == "active")
}

type AbuseStats struct {
	WindowStart        string         `json:"window_start"`
	Reports            int            `json:"reports"`
	OpenReports        int            `json:"open_reports"`
	ReviewingReports   int            `json:"reviewing_reports"`
	ActiveRestrictions int            `json:"active_restrictions"`
	ActiveSuspensions  int            `json:"active_suspensions"`
	ActiveTakedowns    int            `json:"active_takedowns"`
	OpenAppeals        int            `json:"open_appeals"`
	ReportsByReason    map[string]int `json:"reports_by_reason"`
}

func normalizeQueueStatus(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "open", "reviewing", "resolved", "dismissed":
		return value, nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizeModerationState(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "active", "restricted", "suspended", "takedown":
		return value, nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizeResolutionStatus(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "resolved", "dismissed":
		return value, nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizeAppealStatus(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "open", "reviewing", "resolved", "dismissed":
		return value, nil
	default:
		return "", ErrInvalidInput
	}
}

func validateNote(value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > max {
		return "", ErrInvalidInput
	}
	return value, nil
}
