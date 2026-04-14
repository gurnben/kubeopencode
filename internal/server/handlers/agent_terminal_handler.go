// Copyright Contributors to the KubeOpenCode project

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubeopenv1alpha1 "github.com/kubeopencode/kubeopencode/api/v1alpha1"
	"github.com/kubeopencode/kubeopencode/internal/controller"
	authmiddleware "github.com/kubeopencode/kubeopencode/internal/server/middleware"
)

// terminalIdleTimeout is the duration after which an idle terminal session is closed.
// The timeout resets on each WebSocket read (user input).
const terminalIdleTimeout = 30 * time.Minute

// maxExecRetries is the number of times to retry a failed exec session.
// This handles transient failures after agent resume (e.g., exit code 137 when
// the server process is not yet fully initialized).
const maxExecRetries = 3

// execRetryDelay is the delay between exec retry attempts.
const execRetryDelay = 2 * time.Second

var termLog = ctrl.Log.WithName("terminal")

// AgentTerminalHandler handles WebSocket terminal sessions to agent server pods.
type AgentTerminalHandler struct {
	defaultClient    client.Client
	defaultClientset kubernetes.Interface
	restConfig       *rest.Config
}

// NewAgentTerminalHandler creates a new AgentTerminalHandler.
func NewAgentTerminalHandler(c client.Client, clientset kubernetes.Interface, restConfig *rest.Config) *AgentTerminalHandler {
	return &AgentTerminalHandler{
		defaultClient:    c,
		defaultClientset: clientset,
		restConfig:       restConfig,
	}
}

// checkSameOrigin returns true if the Origin header matches the Host header (same-origin).
// Non-browser clients (no Origin header) are allowed.
func checkSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Non-browser clients don't send Origin
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

var upgrader = websocket.Upgrader{
	CheckOrigin: checkSameOrigin,
}

// resizeMessage is a terminal resize control message from the browser.
type resizeMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// terminalSizeQueue implements remotecommand.TerminalSizeQueue.
type terminalSizeQueue struct {
	ch chan *remotecommand.TerminalSize
}

func (q *terminalSizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-q.ch
	if !ok {
		return nil
	}
	return size
}

// ServeTerminal upgrades the HTTP connection to WebSocket and bridges it to
// a pod exec session running "opencode attach" in the agent's server pod.
//
// Includes automatic retry for transient exec failures (e.g., exit code 137)
// that occur when the agent's server is not yet fully initialized after resume.
func (h *AgentTerminalHandler) ServeTerminal(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")
	agentName := chi.URLParam(r, "name")

	k8sClient := clientFromContext(r.Context(), h.defaultClient)

	// Resolve the agent's server pod
	podName, containerName, port, err := resolveAgentServerPod(r.Context(), k8sClient, namespace, agentName)
	if err != nil {
		termLog.Error(err, "failed to resolve agent server pod", "agent", agentName, "namespace", namespace)
		writeError(w, http.StatusBadRequest, "Cannot resolve agent server pod", err.Error())
		return
	}

	// Build impersonated rest.Config for exec RBAC enforcement.
	// This ensures the Kubernetes API server checks the user's pods/exec permission,
	// not the controller's service account.
	execConfig := rest.CopyConfig(h.restConfig)
	userInfo := authmiddleware.GetUserInfo(r.Context())
	if userInfo != nil {
		execConfig.Impersonate = rest.ImpersonationConfig{
			UserName: userInfo.Username,
			UID:      userInfo.UID,
			Groups:   userInfo.Groups,
		}
		termLog.Info("terminal session starting", "user", userInfo.Username, "agent", agentName, "namespace", namespace, "pod", podName)
	}

	// Upgrade to WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		termLog.Error(err, "websocket upgrade failed")
		return
	}
	defer func() { _ = ws.Close() }()

	// Mutex to serialize all WebSocket writes (gorilla/websocket requires this)
	var wsMu sync.Mutex

	// Detach from chi's 60s timeout for long-lived connection.
	// Use a separate cancellable context so we can stop the exec stream and heartbeat
	// when the WebSocket disconnects.
	sessionCtx, sessionCancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer sessionCancel()

	// Start connection heartbeat to prevent standby auto-suspend while terminal is active.
	// Uses the server's service account (defaultClient), not the impersonated user client.
	go controller.RunConnectionHeartbeat(sessionCtx, h.defaultClient, namespace, agentName, func(err error) {
		termLog.Error(err, "heartbeat: failed to patch annotation", "agent", agentName)
	})

	// Build the exec clientset using impersonated config
	execClientset, err := kubernetes.NewForConfig(execConfig)
	if err != nil {
		termLog.Error(err, "failed to create impersonated clientset")
		wsMu.Lock()
		_ = ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "auth failed"))
		wsMu.Unlock()
		return
	}

	// Single WebSocket reader goroutine that persists across exec retry attempts.
	// Input and resize events are sent to channels, which per-attempt pump goroutines
	// forward to the exec session's stdin pipe and terminal size queue.
	inputCh := make(chan []byte, 16)
	resizeCh := make(chan *remotecommand.TerminalSize, 1)

	go func() {
		defer sessionCancel()
		defer close(inputCh)
		defer close(resizeCh)
		_ = ws.SetReadDeadline(time.Now().Add(terminalIdleTimeout))
		for {
			msgType, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			_ = ws.SetReadDeadline(time.Now().Add(terminalIdleTimeout))

			if msgType == websocket.TextMessage {
				var msg resizeMessage
				if err := json.Unmarshal(data, &msg); err != nil {
					continue
				}
				if msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
					select {
					case resizeCh <- &remotecommand.TerminalSize{
						Width:  msg.Cols,
						Height: msg.Rows,
					}:
					default:
					}
				}
			} else {
				select {
				case inputCh <- append([]byte(nil), data...):
				case <-sessionCtx.Done():
					return
				}
			}
		}
	}()

	wsWriter := &wsStdoutWriter{ws: ws, mu: &wsMu}
	attachCmd := buildAttachCommand(containerName, port)
	termLog.Info("resolved attach command", "container", containerName, "command", attachCmd, "agent", agentName)

	// Retry loop for transient exec failures (e.g., exit code 137 after agent resume)
	var lastErr error
	for attempt := 1; attempt <= maxExecRetries; attempt++ {
		if sessionCtx.Err() != nil {
			break
		}

		execReq := execClientset.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(podName).
			Namespace(namespace).
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Container: containerName,
				Command:   attachCmd,
				Stdin:     true,
				Stdout:    true,
				TTY:       true,
			}, scheme.ParameterCodec)

		executor, err := remotecommand.NewSPDYExecutor(execConfig, "POST", execReq.URL())
		if err != nil {
			termLog.Error(err, "failed to create SPDY executor")
			lastErr = err
			break
		}

		// Per-attempt pipe and size queue, with a cancel to stop the pump goroutine
		pr, pw := io.Pipe()
		sizeQueue := &terminalSizeQueue{ch: make(chan *remotecommand.TerminalSize, 1)}
		attemptCtx, attemptCancel := context.WithCancel(sessionCtx)

		// Pump goroutine: reads from shared channels, writes to per-attempt pipe
		var pumpWg sync.WaitGroup
		pumpWg.Add(1)
		go func() {
			defer pumpWg.Done()
			defer func() { _ = pw.Close() }()
			defer close(sizeQueue.ch)
			for {
				select {
				case data, ok := <-inputCh:
					if !ok {
						return
					}
					if _, err := pw.Write(data); err != nil {
						return
					}
				case size, ok := <-resizeCh:
					if !ok {
						return
					}
					select {
					case sizeQueue.ch <- size:
					default:
					}
				case <-attemptCtx.Done():
					return
				}
			}
		}()

		lastErr = executor.StreamWithContext(attemptCtx, remotecommand.StreamOptions{
			Stdin:             pr,
			Stdout:            wsWriter,
			Tty:               true,
			TerminalSizeQueue: sizeQueue,
		})

		attemptCancel()
		_ = pr.Close()
		pumpWg.Wait()

		if lastErr == nil || !isTransientExecError(lastErr) || attempt == maxExecRetries {
			break
		}

		termLog.Info("transient exec failure, retrying",
			"attempt", attempt, "error", lastErr, "agent", agentName)
		wsMu.Lock()
		retryMsg := fmt.Sprintf("\r\n\x1b[33mConnection interrupted, retrying (%d/%d)...\x1b[0m\r\n",
			attempt, maxExecRetries)
		_ = ws.WriteMessage(websocket.BinaryMessage, []byte(retryMsg))
		wsMu.Unlock()

		select {
		case <-time.After(execRetryDelay):
		case <-sessionCtx.Done():
		}
	}

	if lastErr != nil {
		termLog.Info("exec session ended", "error", lastErr, "agent", agentName)
		errMsg := fmt.Sprintf("\r\n\x1b[31mError: %s\x1b[0m\r\n", lastErr.Error())
		wsMu.Lock()
		_ = ws.WriteMessage(websocket.BinaryMessage, []byte(errMsg))
		_ = ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "exec failed"))
		wsMu.Unlock()
	} else {
		wsMu.Lock()
		_ = ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		wsMu.Unlock()
	}
}

// isTransientExecError returns true if the exec error is transient and worth retrying.
// Exit code 137 (SIGKILL) typically occurs when the server process is not yet fully
// initialized after agent resume from standby.
func isTransientExecError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "exit code 137")
}

// wsStdoutWriter writes exec stdout to a WebSocket connection with mutex protection.
type wsStdoutWriter struct {
	ws *websocket.Conn
	mu *sync.Mutex
}

func (w *wsStdoutWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ws.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// resolveAgentServerPod finds the running server pod for a Server-mode Agent.
func resolveAgentServerPod(ctx context.Context, k8sClient client.Client, namespace, agentName string) (podName string, containerName string, port int32, err error) {
	var agent kubeopenv1alpha1.Agent
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: agentName}, &agent); err != nil {
		return "", "", 0, fmt.Errorf("agent not found: %w", err)
	}

	if agent.Status.Suspended {
		return "", "", 0, fmt.Errorf("agent %q is suspended", agentName)
	}
	if !agent.Status.Ready {
		return "", "", 0, fmt.Errorf("agent %q is not ready (deployment may be starting up)", agentName)
	}

	// Find the server pod using the same labels as server_builder.go
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels{
			"app.kubernetes.io/instance":  agentName,
			"app.kubernetes.io/component": "server",
		},
	); err != nil {
		return "", "", 0, fmt.Errorf("failed to list server pods: %w", err)
	}

	// Find a Ready pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				serverPort := controller.GetServerPort(&agent)
				cName := findServerContainer(pod)
				return pod.Name, cName, serverPort, nil
			}
		}
	}

	return "", "", 0, fmt.Errorf("no ready server pod found for agent %q", agentName)
}

func findServerContainer(pod *corev1.Pod) string {
	for _, c := range pod.Spec.Containers {
		if strings.HasSuffix(c.Name, "-server") {
			return c.Name
		}
	}
	if len(pod.Spec.Containers) > 0 {
		return pod.Spec.Containers[0].Name
	}
	return controller.ServerContainerName
}

func buildAttachCommand(containerName string, port int32) []string {
	if strings.HasPrefix(containerName, "crush") {
		return []string{
			"sh", "-c",
			fmt.Sprintf("CRUSH_CLIENT_SERVER=1 /tools/crush --host tcp://localhost:%d", port),
		}
	}
	return []string{"/tools/opencode", "attach", fmt.Sprintf("http://localhost:%d", port)}
}
