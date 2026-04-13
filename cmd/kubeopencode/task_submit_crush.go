// Copyright Contributors to the KubeOpenCode project

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	crushPollTimeout = 30 * time.Minute
)

type crushWorkspaceResponse struct {
	ID string `json:"id"`
}

type crushSessionResponse struct {
	ID string `json:"id"`
}

type crushAgentInfo struct {
	IsBusy  bool `json:"is_busy"`
	IsReady bool `json:"is_ready"`
}

func runCrushTaskSubmit(serverURL, prompt string) error {
	client := &http.Client{Timeout: httpTimeout}

	fmt.Println("[task-submit] creating Crush workspace...")
	workspaceID, err := crushCreateWorkspace(client, serverURL)
	if err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}
	fmt.Printf("[task-submit] workspace created: %s\n", workspaceID)

	fmt.Println("[task-submit] initializing agent...")
	if err := crushInitAgent(client, serverURL, workspaceID); err != nil {
		return fmt.Errorf("failed to init agent: %w", err)
	}

	fmt.Println("[task-submit] enabling skip-permissions...")
	if err := crushSkipPermissions(client, serverURL, workspaceID); err != nil {
		return fmt.Errorf("failed to skip permissions: %w", err)
	}

	fmt.Println("[task-submit] creating session...")
	sessionID, err := crushCreateSession(client, serverURL, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	fmt.Printf("[task-submit] session created: %s\n", sessionID)

	done := make(chan struct{})
	go crushStreamEvents(serverURL, workspaceID, sessionID, done)

	fmt.Println("[task-submit] submitting prompt...")
	if err := crushSubmitPrompt(client, serverURL, workspaceID, sessionID, prompt); err != nil {
		return fmt.Errorf("failed to submit prompt: %w", err)
	}
	fmt.Println("[task-submit] prompt submitted, waiting for completion...")

	if err := crushWaitForIdle(client, serverURL, workspaceID); err != nil {
		return fmt.Errorf("session failed: %w", err)
	}

	close(done)
	fmt.Println("[task-submit] session completed successfully")
	return nil
}

func crushCreateWorkspace(client *http.Client, serverURL string) (string, error) {
	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir == "" {
		workspaceDir = "/workspace"
	}

	payload := map[string]interface{}{
		"path": workspaceDir,
		"yolo": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := client.Post(serverURL+"/v1/workspaces", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result crushWorkspaceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode workspace response: %w", err)
	}

	if result.ID == "" {
		return "", fmt.Errorf("workspace ID is empty")
	}

	return result.ID, nil
}

func crushInitAgent(client *http.Client, serverURL, workspaceID string) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/v1/workspaces/%s/agent/init", serverURL, workspaceID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func crushSkipPermissions(client *http.Client, serverURL, workspaceID string) error {
	resp, err := client.Post(
		fmt.Sprintf("%s/v1/workspaces/%s/permissions/skip", serverURL, workspaceID),
		"application/json",
		bytes.NewBufferString("true"),
	)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func crushCreateSession(client *http.Client, serverURL, workspaceID string) (string, error) {
	payload := map[string]string{
		"title": "kubeopencode-task",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := client.Post(
		fmt.Sprintf("%s/v1/workspaces/%s/sessions", serverURL, workspaceID),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result crushSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode session response: %w", err)
	}

	if result.ID == "" {
		return "", fmt.Errorf("session ID is empty")
	}

	return result.ID, nil
}

func crushSubmitPrompt(client *http.Client, serverURL, workspaceID, sessionID, prompt string) error {
	payload := map[string]string{
		"session_id": sessionID,
		"prompt":     prompt,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := client.Post(
		fmt.Sprintf("%s/v1/workspaces/%s/agent", serverURL, workspaceID),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func crushWaitForIdle(client *http.Client, serverURL, workspaceID string) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	deadline := time.Now().Add(crushPollTimeout)

	for {
		<-ticker.C

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for agent to become idle after %s", crushPollTimeout)
		}

		resp, err := client.Get(fmt.Sprintf("%s/v1/workspaces/%s/agent", serverURL, workspaceID))
		if err != nil {
			fmt.Printf("[task-submit] warning: failed to poll agent status: %v\n", err)
			continue
		}

		var info crushAgentInfo
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			_ = resp.Body.Close()
			fmt.Printf("[task-submit] warning: failed to decode agent status: %v\n", err)
			continue
		}
		_ = resp.Body.Close()

		if !info.IsBusy && info.IsReady {
			return nil
		}
	}
}

func crushStreamEvents(serverURL, workspaceID, sessionID string, done chan struct{}) {
	client := &http.Client{Timeout: 0}

	resp, err := client.Get(fmt.Sprintf("%s/v1/workspaces/%s/events", serverURL, workspaceID))
	if err != nil {
		fmt.Printf("[task-submit] warning: failed to connect to Crush SSE: %v\n", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	for {
		select {
		case <-done:
			return
		default:
		}

		if !scanner.Scan() {
			return
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var event struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "message":
			var msg struct {
				SessionID string `json:"session_id"`
				Parts     []struct {
					Type    string `json:"type"`
					Content string `json:"content"`
				} `json:"parts"`
			}
			if json.Unmarshal(event.Payload.Payload, &msg) == nil {
				if msg.SessionID != "" && msg.SessionID != sessionID {
					continue
				}
				for _, part := range msg.Parts {
					if part.Type == "text" {
						fmt.Print(part.Content)
					}
				}
			}

		case "agent_event":
			var agentEvt struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(event.Payload.Payload, &agentEvt) == nil && agentEvt.Error != "" {
				fmt.Printf("\n[task-submit] agent error: %s\n", agentEvt.Error)
			}
		}
	}
}
