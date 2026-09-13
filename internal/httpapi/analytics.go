package httpapi

import (
	"errors"
	"net/http"
	"strings"

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
			metadata := analytics.MetadataFromHeaders(r.Referer(), r.UserAgent())
			_ = options.Analytics.RecordLinkClick(r.Context(), target.UserID, target.Link.ID, metadata)
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		http.Redirect(w, r, target.Link.URL, http.StatusFound)
	})
}

func recordProfileView(r *http.Request, item profile.Profile, options Options) {
	if options.Analytics == nil || r.Method != http.MethodGet {
		return
	}
	metadata := analytics.MetadataFromHeaders(r.Referer(), r.UserAgent())
	_ = options.Analytics.RecordProfileView(r.Context(), item.ID, metadata)
}
