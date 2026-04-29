package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

//go:embed webapp
var webappFiles embed.FS

type postJSON struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Image    string `json:"image"`
	Content  string `json:"content"`
	URL      string `json:"url"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSONBody(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
		return false
	}
	return true
}

func handleAPIFormat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if !readJSONBody(w, r, 1<<22, &req) {
		return
	}
	lines := strings.Split(req.Text, "\n")
	formatted := formatMultiplePosts(lines)
	writeJSON(w, http.StatusOK, map[string]string{"formatted": strings.Join(formatted, "\n\n")})
}

func handleAPIParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if !readJSONBody(w, r, 1<<22, &req) {
		return
	}
	posts, err := parsePostsString(req.Text)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	out := make([]postJSON, 0, len(posts))
	for _, p := range posts {
		u := p.URL
		if u == "" {
			u = extractURL(p.Content)
		}
		out = append(out, postJSON{
			Title:    p.Title,
			Category: strings.TrimSpace(p.Category),
			Image:    strings.TrimSpace(p.Image),
			Content:  p.Content,
			URL:      u,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": out})
}

func handleAPIOG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if !readJSONBody(w, r, 1<<16, &req) {
		return
	}
	u := strings.TrimSpace(req.URL)
	if u == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url required"})
		return
	}
	img, err := getOGImage(u)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"image": "", "error": err.Error()})
		return
	}
	img = strings.ReplaceAll(img, "&amp;", "&")
	img = strings.ReplaceAll(img, " ", "%20")
	writeJSON(w, http.StatusOK, map[string]string{"image": img})
}

func handleAPIUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		StartFrom int        `json:"startFrom"`
		Posts     []postJSON `json:"posts"`
	}
	if !readJSONBody(w, r, 1<<23, &req) {
		return
	}
	posts := make([]Post, 0, len(req.Posts))
	for _, j := range req.Posts {
		posts = append(posts, Post{
			Title:    j.Title,
			Category: strings.TrimSpace(j.Category),
			Image:    strings.TrimSpace(j.Image),
			Content:  j.Content,
		})
	}
	failedAt, err := UploadPostsFromEditor(req.StartFrom, posts)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       false,
			"failedAt": failedAt,
			"error":    err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "failedAt": -1})
}

func runWebServer() {
	static, err := fs.Sub(webappFiles, "webapp")
	if err != nil {
		logger.Error("web assets: %v", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/format", handleAPIFormat)
	mux.HandleFunc("POST /api/parse", handleAPIParse)
	mux.HandleFunc("POST /api/og", handleAPIOG)
	mux.HandleFunc("POST /api/upload", handleAPIUpload)
	mux.Handle("/", http.FileServer(http.FS(static)))

	addr := "127.0.0.1:8080"
	if p := strings.TrimSpace(os.Getenv("WP_UPLOAD_WEB_ADDR")); p != "" {
		addr = p
	}
	logger.Info("Web UI: http://%s", addr)
	logger.Info("Uses .env from current directory (same as CLI upload).")
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("server: %v", err)
	}
}
