package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"konjiyami/server/assets"
	"konjiyami/server/log"
	serverUI "konjiyami/server/ui"

	"github.com/gorilla/mux"
)

var apiLog = log.NamedLogger("api", "http")

type HTTPServer struct {
	server      *http.Server
	router      *mux.Router
	payloads    *payloadStore
	operators   *operatorStore
	audit       *auditLogger
	payloadsDir string
	artifactDir string
	filesDir    string
	uiDir       string
	devMode     bool
}

func NewHTTPServer(devMode bool) *HTTPServer {
	appDir := assets.GetRootAppDir()
	payloadsDir := filepath.Join(appDir, "payloads")
	artifactDir := filepath.Join(appDir, "artifacts")
	filesDir := filepath.Join(appDir, "files")
	os.MkdirAll(payloadsDir, 0o755)
	os.MkdirAll(artifactDir, 0o755)
	os.MkdirAll(filesDir, 0o755)

	// UI directory: server/ui/public/ relative to the executable's working dir
	uiDir := filepath.Join("server", "ui", "public")
	if _, err := os.Stat(uiDir); os.IsNotExist(err) {
		if exe, err := os.Executable(); err == nil {
			alt := filepath.Join(filepath.Dir(exe), "server", "ui", "public")
			if _, err := os.Stat(alt); err == nil {
				uiDir = alt
			}
		}
	}

	// Initialize JWT secret
	if err := initJWTSecret(); err != nil {
		apiLog.Errorf("Failed to init JWT secret: %v", err)
	}

	// Initialize operator store
	ops := newOperatorStore()
	if err := ops.init(); err != nil {
		apiLog.Errorf("Failed to init operators: %v", err)
	}

	// Initialize audit logger
	audit := newAuditLogger()
	if err := audit.init(); err != nil {
		apiLog.Errorf("Failed to init audit log: %v", err)
	}

	router := mux.NewRouter()
	s := &HTTPServer{
		router:      router,
		payloads:    newPayloadStore(),
		operators:   ops,
		audit:       audit,
		payloadsDir: payloadsDir,
		artifactDir: artifactDir,
		filesDir:    filesDir,
		uiDir:       uiDir,
		devMode:     devMode,
		server: &http.Server{
			Handler:      router,
			ReadTimeout:  0,
			WriteTimeout: 0,
			IdleTimeout:  2 * time.Minute,
		},
	}
	s.restorePayloads()
	s.setupMiddleware()
	s.registerRoutes()
	return s
}

// payloadStore holds generated implant records in memory, persisted to JSON.
type payloadStore struct {
	mu    sync.Mutex
	items map[string]payloadRecord
}

func newPayloadStore() *payloadStore {
	return &payloadStore{items: make(map[string]payloadRecord)}
}

func (p *payloadStore) add(rec payloadRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items[rec.ID] = rec
}

func (p *payloadStore) get(id string) (payloadRecord, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	rec, ok := p.items[id]
	return rec, ok
}

func (p *payloadStore) delete(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.items, id)
}

func (p *payloadStore) list() []payloadRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]payloadRecord, 0, len(p.items))
	for _, rec := range p.items {
		out = append(out, rec)
	}
	return out
}

func (s *HTTPServer) restorePayloads() {
	indexPath := filepath.Join(s.payloadsDir, "payloads.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return
	}
	var list []payloadRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	for _, rec := range list {
		s.payloads.add(rec)
	}
}

func (s *HTTPServer) persistPayloads() {
	list := s.payloads.list()
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(s.payloadsDir, "payloads.json"), data, 0o644)
}

func (s *HTTPServer) Start(host string, port uint16, insecure bool) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	if insecure {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("http listen: %w", err)
		}
		apiLog.Infof("HTTP API + UI listening on http://%s (insecure mode)", addr)
		fmt.Printf("[*] HTTP API + UI on http://%s (insecure)\n", addr)
		go func() {
			if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
				apiLog.Errorf("HTTP server error: %v", err)
			}
		}()
		return nil
	}

	ln, err := createTLSListener(addr)
	if err != nil {
		return fmt.Errorf("https listen: %w", err)
	}
	apiLog.Infof("HTTPS API + UI listening on https://%s", addr)
	fmt.Printf("[*] HTTPS API + UI on https://%s\n", addr)
	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			apiLog.Errorf("HTTPS server error: %v", err)
		}
	}()
	return nil
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.audit.close()
	return s.server.Shutdown(ctx)
}

func (s *HTTPServer) setupMiddleware() {
	s.router.Use(corsMiddleware)
}

func (s *HTTPServer) registerRoutes() {
	// ── Public routes (no auth) ──
	s.router.HandleFunc("/login", s.handleLoginPage).Methods("GET")

	// ── Auth endpoints (no JWT required) ──
	auth := s.router.PathPrefix("/api/v1/auth").Subrouter()
	auth.Use(jsonContentType)
	auth.HandleFunc("/login", s.handleJWTLogin).Methods("POST")
	auth.HandleFunc("/refresh", s.handleJWTRefresh).Methods("POST")

	// ── WebSocket (JWT via query param, no JSON content-type) ──
	s.router.HandleFunc("/api/v1/events", s.handleEvents).Methods("GET")

	// ── API v1 (JWT-protected, JSON content type, audit logged) ──
	v1 := s.router.PathPrefix("/api/v1").Subrouter()
	v1.Use(s.jwtRequired)
	v1.Use(jsonContentType)
	v1.Use(s.audit.middleware)

	// System
	v1.HandleFunc("/health", handleHealth).Methods("GET")
	v1.HandleFunc("/version", handleVersion).Methods("GET")
	v1.HandleFunc("/status", s.handleStatus).Methods("GET")

	// Listeners (jobs)
	v1.HandleFunc("/listeners", handleListListeners).Methods("GET")
	v1.HandleFunc("/listeners", handleCreateListener).Methods("POST")
	v1.HandleFunc("/listeners/{id:[0-9]+}", handleKillListener).Methods("DELETE")

	// Sessions
	v1.HandleFunc("/sessions", handleListSessions).Methods("GET")
	v1.HandleFunc("/sessions/{id}", handleRenameSession).Methods("PATCH")
	v1.HandleFunc("/sessions/{id}", handleKillSession).Methods("DELETE")

	// Jobs (alias)
	v1.HandleFunc("/jobs", handleListListeners).Methods("GET")
	v1.HandleFunc("/jobs/{id:[0-9]+}", handleKillListener).Methods("DELETE")

	// Beacons
	v1.HandleFunc("/beacons", handleListBeacons).Methods("GET")
	v1.HandleFunc("/beacons/{id}", handleRenameBeacon).Methods("PATCH")
	v1.HandleFunc("/beacons/{id}", handleKillBeacon).Methods("DELETE")
	v1.HandleFunc("/beacons/{id}/tasks", handleListBeaconTasks).Methods("GET")
	v1.HandleFunc("/beacons/{id}/tasks", handleCreateBeaconTask).Methods("POST")
	v1.HandleFunc("/beacons/{id}/tasks/{taskId}", handleCancelBeaconTask).Methods("DELETE")

	// Session sub-resources
	v1.HandleFunc("/sessions/{id}/console", handleSessionConsole).Methods("POST")
	v1.HandleFunc("/sessions/{id}/actions", handleSessionActions).Methods("POST")

	// Sprint 8a: Execution endpoints (session = sync, beacon = async)
	v1.HandleFunc("/sessions/{id}/execute-assembly", handleExecAssemblySession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/sideload", handleSideloadSession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/inject", handleInjectSession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/ps", handlePsSession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/getsystem", handleGetSystemSession).Methods("POST")

	v1.HandleFunc("/beacons/{id}/execute-assembly", handleExecAssemblyBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/sideload", handleSideloadBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/inject", handleInjectBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/ps", handlePsBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/getsystem", handleGetSystemBeacon).Methods("POST")

	// Sprint 9: File operations (session = sync, beacon = async)
	v1.HandleFunc("/sessions/{id}/files/ls", handleFileLsSession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/files/cd", handleFileCdSession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/files/pwd", handleFilePwdSession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/files/download", handleFileDownloadSession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/files/upload", handleFileUploadSession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/files/mkdir", handleFileMkdirSession).Methods("POST")
	v1.HandleFunc("/sessions/{id}/files/rm", handleFileRmSession).Methods("POST")

	v1.HandleFunc("/beacons/{id}/files/ls", handleFileLsBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/files/cd", handleFileCdBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/files/pwd", handleFilePwdBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/files/download", handleFileDownloadBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/files/upload", handleFileUploadBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/files/mkdir", handleFileMkdirBeacon).Methods("POST")
	v1.HandleFunc("/beacons/{id}/files/rm", handleFileRmBeacon).Methods("POST")

	// Listener kind alias
	v1.HandleFunc("/listeners/{kind}", handleCreateListenerByKind).Methods("POST")

	// Implants
	v1.HandleFunc("/implants", handleListImplantBuilds).Methods("GET")
	v1.HandleFunc("/implants/generate", handleGenerateImplant).Methods("POST")

	// Payloads
	v1.HandleFunc("/payloads", s.handleListPayloads).Methods("GET")
	v1.HandleFunc("/payloads", s.handleCreatePayload).Methods("POST")
	v1.HandleFunc("/payloads/{id}", s.handleGetPayload).Methods("GET")
	v1.HandleFunc("/payloads/{id}", s.handleDeletePayload).Methods("DELETE")
	v1.HandleFunc("/payloads/{id}/download", s.handleDownloadPayload).Methods("GET")

	// Artifacts
	v1.PathPrefix("/artifacts/").HandlerFunc(s.handleArtifact).Methods("GET")

	// File management (arsenal staging)
	v1.HandleFunc("/upload", s.handleUpload).Methods("POST")
	v1.HandleFunc("/files", s.handleFileList).Methods("GET")
	v1.HandleFunc("/files/{filename}", s.handleFileDelete).Methods("DELETE")

	// Audit log (admin only)
	v1.HandleFunc("/audit", s.handleAuditLog).Methods("GET")

	// Whoami
	v1.HandleFunc("/whoami", s.handleWhoami).Methods("GET")

	// ── Staged files (JWT-protected) ──
	s.router.PathPrefix("/files/").Handler(
		s.jwtRequired(http.StripPrefix("/files/", http.FileServer(http.Dir(s.filesDir)))),
	)

	// ── UI static files (no server-side auth, SPA checks JWT in JS) ──
	s.router.PathPrefix("/").Handler(s.uiHandler())
}

// uiHandler returns a handler that serves static files, falling back to index.html for SPA routes.
func (s *HTTPServer) uiHandler() http.Handler {
	if s.devMode {
		return s.diskUIHandler()
	}
	return s.embedUIHandler()
}

func (s *HTTPServer) diskUIHandler() http.Handler {
	dir := http.Dir(s.uiDir)
	fileServer := http.FileServer(dir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/" {
			cleanPath := strings.TrimPrefix(path, "/")
			if f, err := dir.Open(cleanPath); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFile(w, r, filepath.Join(s.uiDir, "index.html"))
	})
}

func (s *HTTPServer) embedUIHandler() http.Handler {
	subFS, err := fs.Sub(serverUI.PublicFS, "public")
	if err != nil {
		apiLog.Errorf("Failed to sub embedded UI FS: %v", err)
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(subFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if f, err := subFS.Open(path); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

// --- Middleware ---

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"detail": msg,
	})
}
