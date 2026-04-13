// Copyright Contributors to the KubeOpenCode project

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestCrushWorkspaceResponseJSON(t *testing.T) {
	raw := `{"id":"ws-123"}`
	var resp crushWorkspaceResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.ID != "ws-123" {
		t.Errorf("expected ws-123, got %s", resp.ID)
	}
}

func TestCrushSessionResponseJSON(t *testing.T) {
	raw := `{"id":"sess-456"}`
	var resp crushSessionResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.ID != "sess-456" {
		t.Errorf("expected sess-456, got %s", resp.ID)
	}
}

func TestCrushAgentInfoJSON(t *testing.T) {
	raw := `{"is_busy":true,"is_ready":false}`
	var info crushAgentInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if !info.IsBusy {
		t.Error("expected is_busy=true")
	}
	if info.IsReady {
		t.Error("expected is_ready=false")
	}
}

func TestCrushCreateWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspaces" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["yolo"] != true {
			t.Error("expected yolo=true in request body")
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(crushWorkspaceResponse{ID: "ws-test-123"})
	}))
	defer server.Close()

	client := &http.Client{Timeout: httpTimeout}
	id, err := crushCreateWorkspace(client, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ws-test-123" {
		t.Errorf("expected ws-test-123, got %s", id)
	}
}

func TestCrushCreateWorkspaceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: httpTimeout}
	_, err := crushCreateWorkspace(client, server.URL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCrushSubmitPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspaces/ws-1/agent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body["session_id"] != "sess-1" {
			t.Errorf("expected session_id=sess-1, got %s", body["session_id"])
		}
		if body["prompt"] != "do the thing" {
			t.Errorf("expected prompt='do the thing', got %s", body["prompt"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: httpTimeout}
	err := crushSubmitPrompt(client, server.URL, "ws-1", "sess-1", "do the thing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCrushSubmitPromptError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: httpTimeout}
	err := crushSubmitPrompt(client, server.URL, "ws-1", "sess-1", "prompt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCrushWaitForIdle(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspaces/ws-1/agent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		count := atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)

		if count < 3 {
			_ = json.NewEncoder(w).Encode(crushAgentInfo{IsBusy: true, IsReady: false})
		} else {
			_ = json.NewEncoder(w).Encode(crushAgentInfo{IsBusy: false, IsReady: true})
		}
	}))
	defer server.Close()

	client := &http.Client{Timeout: httpTimeout}
	err := crushWaitForIdle(client, server.URL, "ws-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount < 3 {
		t.Errorf("expected at least 3 polls, got %d", finalCount)
	}
}

func TestCrushInitAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspaces/ws-1/agent/init" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: httpTimeout}
	err := crushInitAgent(client, server.URL, "ws-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCrushSkipPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspaces/ws-1/permissions/skip" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Timeout: httpTimeout}
	err := crushSkipPermissions(client, server.URL, "ws-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCrushCreateSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspaces/ws-1/sessions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if body["title"] != "kubeopencode-task" {
			t.Errorf("expected title=kubeopencode-task, got %s", body["title"])
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(crushSessionResponse{ID: "sess-test-789"})
	}))
	defer server.Close()

	client := &http.Client{Timeout: httpTimeout}
	id, err := crushCreateSession(client, server.URL, "ws-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "sess-test-789" {
		t.Errorf("expected sess-test-789, got %s", id)
	}
}

func TestCrushCreateWorkspaceEmptyID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"id":""}`)
	}))
	defer server.Close()

	client := &http.Client{Timeout: httpTimeout}
	_, err := crushCreateWorkspace(client, server.URL)
	if err == nil {
		t.Fatal("expected error for empty workspace ID")
	}
}
