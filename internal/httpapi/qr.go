package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/chrisbirster/vutame/internal/profile"
	qrcode "github.com/skip2/go-qrcode"
)

func registerProfileUtilityRoutes(mux *http.ServeMux, profiles profile.Store, options Options) {
	mux.HandleFunc("GET /api/v1/profiles/{handle}/qr.png", func(w http.ResponseWriter, r *http.Request) {
		item, err := profiles.Get(r.PathValue("handle"))
		if errors.Is(err, profile.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		origin := strings.TrimRight(options.ProfileOrigin, "/")
		canonical := origin + "/@" + url.PathEscape(item.Handle)
		png, err := qrcode.Encode(canonical, qrcode.Medium, 512)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "QR generation failed"})
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="vutame-%s-qr.png"`, item.Handle))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(png)
		}
	})
}
