package httpserver

import (
	"net/http"
	"strings"
)

const viewCookieName = "minixiangqi_view"

// RegisterStaticRoutes mounts:
// - /web/*        -> desktop assets
// - /web_mobile/* -> mobile assets
// - /             -> auto redirect by view override/cookie/User-Agent
func RegisterStaticRoutes(mux *http.ServeMux, desktopDir string, mobileDir string) {
	if mux == nil {
		return
	}
	if desktopDir == "" {
		desktopDir = "."
	}
	if mobileDir == "" {
		mobileDir = desktopDir
	}

	mux.Handle("/web/", http.StripPrefix("/web/", http.FileServer(http.Dir(desktopDir))))
	mux.Handle("/web_mobile/", http.StripPrefix("/web_mobile/", http.FileServer(http.Dir(mobileDir))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			target := "/web/"
			if pickView(w, r) == "mobile" {
				target = "/web_mobile/"
			}
			w.Header().Set("Vary", "User-Agent, Cookie")
			http.Redirect(w, r, target, http.StatusFound)
			return
		case "/web":
			http.Redirect(w, r, "/web/", http.StatusFound)
			return
		case "/web_mobile":
			http.Redirect(w, r, "/web_mobile/", http.StatusFound)
			return
		default:
			http.NotFound(w, r)
			return
		}
	})
}

func pickView(w http.ResponseWriter, r *http.Request) string {
	if v, ok := normalizeView(r.URL.Query().Get("view")); ok {
		rememberView(w, v)
		return v
	}

	if c, err := r.Cookie(viewCookieName); err == nil {
		if v, ok := normalizeView(c.Value); ok {
			return v
		}
	}

	if isMobileUA(r.UserAgent()) {
		return "mobile"
	}
	return "web"
}

func rememberView(w http.ResponseWriter, view string) {
	http.SetCookie(w, &http.Cookie{
		Name:     viewCookieName,
		Value:    view,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
	})
}

func normalizeView(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "web", "desktop", "pc":
		return "web", true
	case "mobile", "m", "phone", "web_mobile":
		return "mobile", true
	default:
		return "", false
	}
}

func isMobileUA(ua string) bool {
	s := strings.ToLower(ua)
	if s == "" {
		return false
	}
	needles := []string{
		"android",
		"iphone",
		"ipad",
		"ipod",
		"mobile",
		"windows phone",
		"harmony",
	}
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}
