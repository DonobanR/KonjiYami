package api

import (
	"encoding/json"
	"net/http"
	"time"

	consts "konjiyami/client/constants"
	"konjiyami/server/core"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Data      any    `json:"data,omitempty"`
}

func (s *HTTPServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = extractBearerToken(r)
	}
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	claims, err := validateToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		apiLog.Errorf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	events := core.EventBroker.Subscribe()
	defer core.EventBroker.Unsubscribe(events)

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()

	_ = claims

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return
			}
			msg := translateEvent(evt)
			if msg == nil {
				continue
			}
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(msg); err != nil {
				return
			}

		case <-ping.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func translateEvent(evt core.Event) *wsEvent {
	now := time.Now().UTC().Format(time.RFC3339)
	switch evt.EventType {
	case consts.SessionOpenedEvent:
		data := map[string]any{}
		if evt.Session != nil {
			data["id"] = evt.Session.ID
			data["name"] = evt.Session.Name
			data["hostname"] = evt.Session.Hostname
			data["username"] = evt.Session.Username
			data["os"] = evt.Session.OS
			data["arch"] = evt.Session.Arch
			if evt.Session.Connection != nil {
				data["remoteAddress"] = evt.Session.Connection.RemoteAddress
			}
		}
		return &wsEvent{Type: "session.new", Timestamp: now, Data: data}

	case consts.SessionClosedEvent:
		data := map[string]any{}
		if evt.Session != nil {
			data["id"] = evt.Session.ID
		}
		return &wsEvent{Type: "session.dead", Timestamp: now, Data: data}

	case consts.BeaconRegisteredEvent:
		data := map[string]any{}
		if evt.Beacon != nil {
			data["id"] = evt.Beacon.ID.String()
			data["name"] = evt.Beacon.Name
			data["hostname"] = evt.Beacon.Hostname
			data["username"] = evt.Beacon.Username
			data["os"] = evt.Beacon.OS
			data["arch"] = evt.Beacon.Arch
		}
		return &wsEvent{Type: "beacon.checkin", Timestamp: now, Data: data}

	case consts.JobStartedEvent:
		data := map[string]any{}
		if evt.Job != nil {
			data["id"] = evt.Job.ID
			data["name"] = evt.Job.Name
			data["protocol"] = evt.Job.Protocol
			data["port"] = evt.Job.Port
		}
		return &wsEvent{Type: "listener.started", Timestamp: now, Data: data}

	case consts.JobStoppedEvent:
		data := map[string]any{}
		if evt.Job != nil {
			data["id"] = evt.Job.ID
			data["name"] = evt.Job.Name
		}
		return &wsEvent{Type: "listener.stopped", Timestamp: now, Data: data}

	case consts.SessionUpdateEvent:
		data := map[string]any{}
		if evt.Session != nil {
			data["id"] = evt.Session.ID
			data["name"] = evt.Session.Name
			data["hostname"] = evt.Session.Hostname
			data["username"] = evt.Session.Username
		}
		return &wsEvent{Type: "session.updated", Timestamp: now, Data: data}

	case "beacon-renamed":
		data := map[string]any{}
		if evt.Beacon != nil {
			data["id"] = evt.Beacon.ID.String()
			data["name"] = evt.Beacon.Name
			data["hostname"] = evt.Beacon.Hostname
			data["username"] = evt.Beacon.Username
		}
		return &wsEvent{Type: "beacon.renamed", Timestamp: now, Data: data}

	case consts.BeaconTaskResultEvent:
		data := map[string]any{}
		if len(evt.Data) > 0 {
			var parsed any
			if json.Unmarshal(evt.Data, &parsed) == nil {
				data["task"] = parsed
			}
		}
		return &wsEvent{Type: "beacon.taskresult", Timestamp: now, Data: data}

	case consts.BuildCompletedEvent:
		data := map[string]any{}
		if len(evt.Data) > 0 {
			data["name"] = string(evt.Data)
		}
		return &wsEvent{Type: "build.completed", Timestamp: now, Data: data}

	default:
		if evt.EventType == "" {
			return nil
		}
		data := map[string]any{"raw": evt.EventType}
		if len(evt.Data) > 0 {
			var parsed any
			if json.Unmarshal(evt.Data, &parsed) == nil {
				data["payload"] = parsed
			} else {
				data["payload"] = string(evt.Data)
			}
		}
		return &wsEvent{Type: evt.EventType, Timestamp: now, Data: data}
	}
}
