package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrisbirster/vutame/internal/analytics"
	"github.com/chrisbirster/vutame/internal/profile"
)

func registerAnalyticsRoutes(mux *http.ServeMux, profiles profile.Store, options Options) {
	mux.HandleFunc("GET /out/{linkID}", func(w http.ResponseWriter, r *http.Request) {
		resolver, ok := profiles.(profile.PublicLinkResolver)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "link not found"})
			return
		}
		target, err := resolver.ResolvePublicLink(strings.TrimSpace(r.PathValue("linkID")))
		if errors.Is(err, profile.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "link not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		if options.Analytics != nil {
			metadata := requestAnalyticsMetadata(r, target.UserID, options.Analytics)
			_ = options.Analytics.RecordLinkClick(r.Context(), target.UserID, target.Link.ID, metadata)
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		http.Redirect(w, r, target.Link.URL, http.StatusFound)
	})

	mux.HandleFunc("GET /api/v1/me/analytics", func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAuthenticatedUser(w, r, options)
		if !ok {
			return
		}
		if options.Analytics == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "analytics unavailable"})
			return
		}
		days := analytics.DefaultDashboardDays
		if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 || parsed > analytics.MaxDashboardDays {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be between 1 and 90"})
				return
			}
			days = parsed
		}
		dashboard, err := options.Analytics.Dashboard(r.Context(), user.ID, days, time.Now().UTC())
		if errors.Is(err, analytics.ErrProfileRequired) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "claim a Vuta before viewing analytics"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, dashboard)
	})
}

func recordProfileView(r *http.Request, item profile.Profile, options Options) {
	if options.Analytics == nil || r.Method != http.MethodGet {
		return
	}
	metadata := requestAnalyticsMetadata(r, item.ID, options.Analytics)
	_ = options.Analytics.RecordProfileView(r.Context(), item.ID, metadata)
}

func requestAnalyticsMetadata(r *http.Request, userID string, service *analytics.Service) analytics.Metadata {
	metadata := analytics.MetadataFromHeaders(r.Referer(), r.UserAgent())
	clientIP := analytics.ClientIP(r.Header.Get("Fly-Client-IP"), r.RemoteAddr)
	metadata.VisitorHash = service.VisitorToken(userID, clientIP, time.Now().UTC())
	return metadata
}
