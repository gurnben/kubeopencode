// Copyright Contributors to the KubeOpenCode project

package controller

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
)

// RuntimeProfile encapsulates all runtime-specific behavior for a coding agent.
// This avoids scattering runtime conditionals throughout the controller.
type RuntimeProfile struct {
	// Name is the runtime identifier ("opencode" or "crush").
	Name string

	// BinaryName is the binary filename in /tools (e.g., "opencode", "crush").
	BinaryName string

	// ConfigFileName is the config file name (e.g., "opencode.json", "crush.json").
	ConfigFileName string

	// ConfigEnvVar is the env var that points to the config file path.
	// Empty string means the runtime uses file discovery instead.
	ConfigEnvVar string

	// ConfigContentEnvVar is the env var for injecting additional config content.
	// Empty string means the runtime doesn't support this mechanism.
	ConfigContentEnvVar string

	// PermissionEnvVar is the env var for permission bypass.
	// Empty string means the runtime uses CLI flags instead.
	PermissionEnvVar string

	// DefaultPermissionValue is the value for the permission env var.
	DefaultPermissionValue string

	// PermissionFlags are CLI flags appended to bypass interactive prompts.
	// For Crush this is ["--yolo"], for OpenCode this is empty (uses env var).
	PermissionFlags []string

	// DefaultAgentImage is the default init container image for this runtime.
	DefaultAgentImage string

	// HealthPath is the HTTP health check path for the server mode.
	HealthPath string

	// DBEnvVar is the env var for overriding the session database path.
	// Empty string means the runtime manages its own DB location.
	DBEnvVar string

	// InitContainerName is the name used for the init container.
	InitContainerName string

	// ServerContainerName is the name used for the server container.
	ServerContainerName string

	// SessionDBName is the database filename for session persistence (e.g., "opencode.db").
	SessionDBName string

	// ExtraEnvVars are additional env vars to inject into all containers.
	ExtraEnvVars []corev1.EnvVar
}

// BuildRunCommand constructs the shell command for running a task.
// serverURL is empty for templateRef tasks (no server to attach to).
func (p *RuntimeProfile) BuildRunCommand(workspaceDir, taskTitle, serverURL string) string {
	binary := fmt.Sprintf("/tools/%s", p.BinaryName)
	promptArg := fmt.Sprintf(`"$(cat %s/task.md)"`, workspaceDir)
	globalFlags := ""
	for _, f := range p.PermissionFlags {
		globalFlags += " " + f
	}

	if serverURL != "" {
		switch p.Name {
		case "opencode":
			return fmt.Sprintf(`%s run --attach %s --title %s %s`, binary, serverURL, taskTitle, promptArg)
		case "crush":
			return fmt.Sprintf(`%s%s run %s`, binary, globalFlags, promptArg)
		default:
			return fmt.Sprintf(`%s%s run %s`, binary, globalFlags, promptArg)
		}
	}

	switch p.Name {
	case "opencode":
		return fmt.Sprintf(`%s run --title %s %s`, binary, taskTitle, promptArg)
	case "crush":
		return fmt.Sprintf(`%s%s run %s`, binary, globalFlags, promptArg)
	default:
		return fmt.Sprintf(`%s%s run %s`, binary, globalFlags, promptArg)
	}
}

// BuildServeCommand constructs the shell command for running the server.
func (p *RuntimeProfile) BuildServeCommand(port int) string {
	binary := fmt.Sprintf("/tools/%s", p.BinaryName)
	switch p.Name {
	case "opencode":
		return fmt.Sprintf(`%s serve --port %d --hostname 0.0.0.0`, binary, port)
	case "crush":
		return fmt.Sprintf(`%s server --host tcp://0.0.0.0:%d`, binary, port)
	default:
		return fmt.Sprintf(`%s serve --port %d --hostname 0.0.0.0`, binary, port)
	}
}

// ConfigPath returns the full path to the config file in /tools.
func (p *RuntimeProfile) ConfigPath() string {
	if p.Name == "crush" {
		return "/tmp/crush-data/crush.json"
	}
	return fmt.Sprintf("/tools/%s", p.ConfigFileName)
}

// SymlinkCmd returns a shell command that creates a symlink for the runtime
// binary in /usr/local/bin so it is discoverable from interactive terminals.
func (p *RuntimeProfile) SymlinkCmd() string {
	return fmt.Sprintf("ln -sf /tools/%s /usr/local/bin/%s 2>/dev/null || true", p.BinaryName, p.BinaryName)
}

// Profiles registry
var runtimeProfiles = map[string]*RuntimeProfile{
	"opencode": {
		Name:                   "opencode",
		BinaryName:             "opencode",
		ConfigFileName:         "opencode.json",
		ConfigEnvVar:           "OPENCODE_CONFIG",
		ConfigContentEnvVar:    "OPENCODE_CONFIG_CONTENT",
		PermissionEnvVar:       "OPENCODE_PERMISSION",
		DefaultPermissionValue: `{"*":"allow"}`,
		PermissionFlags:        nil,
		DefaultAgentImage:      "ghcr.io/kubeopencode/kubeopencode-agent-opencode:latest",
		HealthPath:             "/session/status",
		DBEnvVar:               "OPENCODE_DB",
		InitContainerName:      "opencode-init",
		ServerContainerName:    "opencode-server",
		SessionDBName:          "opencode.db",
		ExtraEnvVars:           nil,
	},
	"crush": {
		Name:                   "crush",
		BinaryName:             "crush",
		ConfigFileName:         "crush.json",
		ConfigEnvVar:           "",
		ConfigContentEnvVar:    "",
		PermissionEnvVar:       "",
		DefaultPermissionValue: "",
		PermissionFlags:        nil, // crush run is non-interactive and auto-accepts permissions
		DefaultAgentImage:      "ghcr.io/gurnben/crush-container:latest",
		HealthPath:             "/v1/health",
		DBEnvVar:               "",
		InitContainerName:      "crush-init",
		ServerContainerName:    "crush-server",
		SessionDBName:          "crush.db",
		ExtraEnvVars: []corev1.EnvVar{
			{Name: "CRUSH_DISABLE_METRICS", Value: "1"},
			{Name: "CRUSH_DISABLE_PROVIDER_AUTO_UPDATE", Value: "1"},
			{Name: "DO_NOT_TRACK", Value: "1"},
			{Name: "CRUSH_GLOBAL_DATA", Value: "/tmp/crush-data"},
		},
	},
}

// GetRuntimeProfile returns the RuntimeProfile for the given runtime name.
// Defaults to "opencode" if the name is empty or unknown.
func GetRuntimeProfile(runtime string) *RuntimeProfile {
	if runtime == "" {
		runtime = "opencode"
	}
	if p, ok := runtimeProfiles[runtime]; ok {
		return p
	}
	return runtimeProfiles["opencode"]
}
