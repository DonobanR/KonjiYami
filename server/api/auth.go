package api

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	serverUI "konjiyami/server/ui"
	"github.com/gorilla/mux"
)

// ── JWT Login / Refresh handlers ──────────────────────────

func (s *HTTPServer) handleJWTLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}

	op, err := s.operators.authenticate(req.Username, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	accessToken, err := generateAccessToken(op.Username, op.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	refreshToken, err := generateRefreshToken(op.Username, op.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"expires_in":    int(accessTokenExpiry.Seconds()),
		"username":      op.Username,
		"role":          op.Role,
	})
}

func (s *HTTPServer) handleJWTRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims, err := validateToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if claims.Type != "refresh" {
		writeError(w, http.StatusUnauthorized, "not a refresh token")
		return
	}

	accessToken, err := generateAccessToken(claims.Username, claims.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   int(accessTokenExpiry.Seconds()),
	})
}

// ── Login page handler ──────────────────────────

func (s *HTTPServer) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if s.devMode {
		http.ServeFile(w, r, filepath.Join(s.uiDir, "login.html"))
		return
	}
	subFS, err := fs.Sub(serverUI.PublicFS, "public")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(subFS, "login.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// ── Whoami handler (JWT-protected) ──────────────────────────

func (s *HTTPServer) handleWhoami(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, map[string]string{
		"username": claims.Username,
		"role":     claims.Role,
	})
}

// ── File upload handler ────────────────

func (s *HTTPServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	const maxUploadSize = 100 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	dstPath := filepath.Join(s.filesDir, filepath.Base(header.Filename))
	dst, err := os.Create(dstPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create file")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save file")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"filename": header.Filename,
		"size":     header.Size,
	})
}

func (s *HTTPServer) handleFileList(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.filesDir)
	if err != nil {
		writeJSON(w, http.StatusOK, []string{})
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	writeJSON(w, http.StatusOK, names)
}

func (s *HTTPServer) handleFileDelete(w http.ResponseWriter, r *http.Request) {
	filename := mux.Vars(r)["filename"]
	if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	target := filepath.Join(s.filesDir, filename)
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "file not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to delete file")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
