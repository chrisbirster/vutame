package safety

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrNotFound       = errors.New("creator not found")
	ErrSelfAction     = errors.New("cannot apply safety action to yourself")
	ErrProfileRequired = errors.New("profile required")
	ErrInvalidReport  = errors.New("invalid report")
	ErrInvalidState   = errors.New("invalid moderation state")
)

const MaxReportDetail = 1000

type Privacy struct {
	Discoverable    bool `json:"discoverable"`
	ActivityVisible bool `json:"activity_visible"`
	AllowFollows    bool `json:"allow_follows"`
}

type Relation struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

type ReportInput struct {
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

type Report struct {
	ID             string `json:"id"`
	ReportedHandle string `json:"reported_handle"`
	Reason         string `json:"reason"`
	Detail         string `json:"detail,omitempty"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

type Moderation struct {
	State string `json:"state"`
	Note  string `json:"note,omitempty"`
}

type Store interface {
	Block(context.Context, string, string) error
	Unblock(context.Context, string, string) error
	Mute(context.Context, string, string) error
	Unmute(context.Context, string, string) error
	Blocks(context.Context, string) ([]Relation, error)
	Mutes(context.Context, string) ([]Relation, error)
	Privacy(context.Context, string) (Privacy, error)
	UpdatePrivacy(context.Context, string, Privacy) (Privacy, error)
	Report(context.Context, string, string, ReportInput) (Report, error)
	SetModeration(context.Context, string, Moderation) error
	Moderation(context.Context, string) (Moderation, error)
}

func DefaultPrivacy() Privacy {
	return Privacy{Discoverable: true, ActivityVisible: true, AllowFollows: true}
}

func ValidateReport(input ReportInput) (ReportInput, error) {
	input.Reason = strings.ToLower(strings.TrimSpace(input.Reason))
	input.Detail = strings.TrimSpace(input.Detail)
	switch input.Reason {
	case "spam", "harassment", "impersonation", "unsafe", "other":
	default:
		return ReportInput{}, fmt.Errorf("%w: unsupported reason", ErrInvalidReport)
	}
	if utf8.RuneCountInString(input.Detail) > MaxReportDetail {
		return ReportInput{}, fmt.Errorf("%w: detail must be %d characters or fewer", ErrInvalidReport, MaxReportDetail)
	}
	return input, nil
}

func ValidateModeration(input Moderation) (Moderation, error) {
	input.State = strings.ToLower(strings.TrimSpace(input.State))
	input.Note = strings.TrimSpace(input.Note)
	switch input.State {
	case "active", "restricted", "suspended":
	default:
		return Moderation{}, ErrInvalidState
	}
	if utf8.RuneCountInString(input.Note) > 1000 {
		return Moderation{}, fmt.Errorf("%w: moderation note is too long", ErrInvalidState)
	}
	return input, nil
}
