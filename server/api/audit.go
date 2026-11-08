package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"konjiyami/server/assets"
)

type auditEntry struct {
	Timestamp string `json:"timestamp"`
	User      string `json:"user"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	IP        string `json:"ip"`
	Duration  string `json:"duration"`
}

type auditLogger struct {
	mu       sync.Mutex
	filePath string
	file     *os.File
	writer   *bufio.Writer
}

func newAuditLogger() *auditLogger {
	return &auditLogger{
		filePath: filepath.Join(assets.GetRootAppDir(), "audit.jsonl"),
	}
}

func (al *auditLogger) init() error {
	f, err := os.OpenFile(al.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	al.file = f
	al.writer = bufio.NewWriter(f)
	return nil
}

func (al *auditLogger) log(entry auditEntry) {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.writer == nil {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	al.writer.Write(data)
	al.writer.WriteByte('\n')
	al.writer.Flush()
}

func (al *auditLogger) close() {
	al.mu.Lock()
	defer al.mu.Unlock()
	if al.writer != nil {
		al.writer.Flush()
	}
	if al.file != nil {
		al.file.Close()
	}
}

// statusWriter captures the HTTP status code written by handlers.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// middleware records every API request to the audit JSONL log.
func (al *auditLogger) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sw, r)

		user := "anonymous"
		if claims := getClaims(r); claims != nil {
			user = claims.Username
		}

		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}

		al.log(auditEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			User:      user,
			Method:    r.Method,
			Path:      r.URL.Path,
			Status:    sw.status,
			IP:        ip,
			Duration:  fmt.Sprintf("%.3fms", float64(time.Since(start).Microseconds())/1000),
		})
	})
}

// handleAuditLog returns the last 100 audit entries. Admin role required.
func (s *HTTPServer) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil || claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	f, err := os.Open(s.audit.filePath)
	if err != nil {
		writeJSON(w, http.StatusOK, []auditEntry{})
		return
	}
	defer f.Close()

	var entries []auditEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e auditEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err == nil {
			entries = append(entries, e)
		}
	}

	if len(entries) > 100 {
		entries = entries[len(entries)-100:]
	}

	writeJSON(w, http.StatusOK, entries)
}
