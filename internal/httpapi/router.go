package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/chrisbirster/vutame/internal/activity"
	"github.com/chrisbirster/vutame/internal/analytics"
	"github.com/chrisbirster/vutame/internal/atproto"
	"github.com/chrisbirster/vutame/internal/auth"
	"github.com/chrisbirster/vutame/internal/billing"
	"github.com/chrisbirster/vutame/internal/growth"
	"github.com/chrisbirster/vutame/internal/linkpreview"
	"github.com/chrisbirster/vutame/internal/media"
	"github.com/chrisbirster/vutame/internal/moderation"
	"github.com/chrisbirster/vutame/internal/operations"
	"github.com/chrisbirster/vutame/internal/profile"
	"github.com/chrisbirster/vutame/internal/ratelimit"
	"github.com/chrisbirster/vutame/internal/safety"
	"github.com/chrisbirster/vutame/internal/social"
)

type Options struct {
	MarketingOrigin string
	ProfileOrigin   string
	Auth            *auth.Service
	Editor          profile.Editor
	Media           *media.Service
	Previewer       linkpreview.Fetcher
	Social          social.Store
	Activity        activity.Store
	Analytics       *analytics.Service
	Growth          *growth.Service
	Operations      *operations.Store
	Safety          safety.Store
	Moderation      *moderation.Service
	ATProto         *atproto.Store
	Billing         *billing.Service
	Limiter         ratelimit.Gate
	CookieSecure    bool
}

func New(web http.Handler, profiles profile.Store, options Options) http.Handler {
	if profiles == nil {
		panic("httpapi: profile store is required")
	}
	options = normalizeOptions(options)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"product":          "vutame",
			"marketing_origin": options.MarketingOrigin,
			"profile_origin":   options.ProfileOrigin,
		})
	})

	mux.HandleFunc("GET /api/v1/discover", func(w http.ResponseWriter, r *http.Request) {
		items, err := profiles.Discover()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		if options.Moderation != nil {
			filtered := make([]profile.Profile, 0, len(items))
			for _, item := range items {
				allowed, err := publicProfileAllowed(r.Context(), options, item.Handle, true)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
					return
				}
				if allowed {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		writeJSON(w, http.StatusOK, map[string]any{"profiles": items})
	})

	mux.HandleFunc("GET /api/v1/handles/{handle}/availability", func(w http.ResponseWriter, r *http.Request) {
		handle := profile.NormalizeHandle(r.PathValue("handle"))
		available, err := profiles.HandleAvailable(handle)
		if errors.Is(err, profile.ErrInvalidHandle) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"handle": handle, "available": available})
	})

	mux.HandleFunc("GET /api/v1/profiles/{handle}", func(w http.ResponseWriter, r *http.Request) {
		handle := strings.TrimSpace(r.PathValue("handle"))
		allowed, err := publicProfileAllowed(r.Context(), options, handle, false)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}
		item, err := profiles.Get(handle)
		if errors.Is(err, profile.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	})

	registerAuthRoutes(mux, options)
	registerEditorRoutes(mux, options)
	registerMediaRoutes(mux, options)
	registerLinkPreviewRoutes(mux, options)
	registerSocialRoutes(mux, profiles, options)
	registerActivityRoutes(mux, options)
	registerSafetyRoutes(mux, options)
	registerModerationRoutes(mux, options)
	registerAnalyticsRoutes(mux, profiles, options)
	registerGrowthRoutes(mux, options)
	registerOperationsRoutes(mux, options)
	registerATProtoRoutes(mux, options)
	registerBillingRoutes(mux, options)
	registerProfileUtilityRoutes(mux, profiles, options)
	mux.Handle("/", profileHTMLHandler(web, profiles, options))
	return mux
}

func normalizeOptions(options Options) Options {
	options.MarketingOrigin = strings.TrimRight(strings.TrimSpace(options.MarketingOrigin), "/")
	options.ProfileOrigin = strings.TrimRight(strings.TrimSpace(options.ProfileOrigin), "/")
	if options.MarketingOrigin == "" {
		options.MarketingOrigin = "https://vutame.com"
	}
	if options.ProfileOrigin == "" {
		options.ProfileOrigin = "https://vuta.me"
	}
	if options.Previewer == nil {
		options.Previewer = linkpreview.NewService()
	}
	if options.Limiter == nil {
		options.Limiter = ratelimit.New()
	}
	return options
}

func publicProfileAllowed(ctx context.Context, options Options, handle string, discovery bool) (bool, error) {
	if options.Moderation == nil {
		return true, nil
	}
	policy, err := options.Moderation.Policy(ctx, handle)
	if errors.Is(err, moderation.ErrNotFound) {
		// The profile store can be an in-memory/seed implementation in development
		// while moderation is backed by the persistent database. Missing policy is
		// therefore not treated as a takedown.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if discovery {
		return policy.Discoverable(), nil
	}
	return !policy.Hidden(), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
