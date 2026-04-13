// Copyright Contributors to the KubeOpenCode project

package controller

import (
	"testing"
)

func TestGetRuntimeProfile_OpenCode(t *testing.T) {
	p := GetRuntimeProfile("opencode")
	if p.Name != "opencode" {
		t.Errorf("expected name 'opencode', got %q", p.Name)
	}
	if p.BinaryName != "opencode" {
		t.Errorf("expected binary 'opencode', got %q", p.BinaryName)
	}
	if p.DefaultAgentImage != "ghcr.io/kubeopencode/kubeopencode-agent-opencode:latest" {
		t.Errorf("unexpected default agent image: %q", p.DefaultAgentImage)
	}
	if p.HealthPath != "/session/status" {
		t.Errorf("expected health path '/session/status', got %q", p.HealthPath)
	}
	if p.DBEnvVar != "OPENCODE_DB" {
		t.Errorf("expected DB env var 'OPENCODE_DB', got %q", p.DBEnvVar)
	}
	if p.ConfigEnvVar != "OPENCODE_CONFIG" {
		t.Errorf("expected config env var 'OPENCODE_CONFIG', got %q", p.ConfigEnvVar)
	}
	if p.PermissionEnvVar != "OPENCODE_PERMISSION" {
		t.Errorf("expected permission env var 'OPENCODE_PERMISSION', got %q", p.PermissionEnvVar)
	}
	if len(p.PermissionFlags) != 0 {
		t.Errorf("expected no permission flags for opencode, got %v", p.PermissionFlags)
	}
}

func TestGetRuntimeProfile_Crush(t *testing.T) {
	p := GetRuntimeProfile("crush")
	if p.Name != "crush" {
		t.Errorf("expected name 'crush', got %q", p.Name)
	}
	if p.BinaryName != "crush" {
		t.Errorf("expected binary 'crush', got %q", p.BinaryName)
	}
	if p.DefaultAgentImage != "ghcr.io/gurnben/crush-container:latest" {
		t.Errorf("unexpected default agent image: %q", p.DefaultAgentImage)
	}
	if p.HealthPath != "/v1/health" {
		t.Errorf("expected health path '/v1/health', got %q", p.HealthPath)
	}
	if p.ConfigEnvVar != "" {
		t.Errorf("expected empty config env var for crush, got %q", p.ConfigEnvVar)
	}
	if p.PermissionEnvVar != "" {
		t.Errorf("expected empty permission env var for crush, got %q", p.PermissionEnvVar)
	}
	if len(p.PermissionFlags) != 1 || p.PermissionFlags[0] != "--yolo" {
		t.Errorf("expected permission flags ['--yolo'], got %v", p.PermissionFlags)
	}
	if len(p.ExtraEnvVars) != 3 {
		t.Errorf("expected 3 extra env vars for crush, got %d", len(p.ExtraEnvVars))
	}
}

func TestGetRuntimeProfile_EmptyDefaultsToOpenCode(t *testing.T) {
	p := GetRuntimeProfile("")
	if p.Name != "opencode" {
		t.Errorf("expected empty string to default to 'opencode', got %q", p.Name)
	}
}

func TestGetRuntimeProfile_UnknownDefaultsToOpenCode(t *testing.T) {
	p := GetRuntimeProfile("unknown")
	if p.Name != "opencode" {
		t.Errorf("expected unknown runtime to default to 'opencode', got %q", p.Name)
	}
}

func TestBuildRunCommand_OpenCode_WithServerURL(t *testing.T) {
	p := GetRuntimeProfile("opencode")
	cmd := p.BuildRunCommand("/workspace", "my-task", "http://server:4096")
	expected := `/tools/opencode run --attach http://server:4096 --title my-task "$(cat /workspace/task.md)"`
	if cmd != expected {
		t.Errorf("unexpected command:\n  got:  %s\n  want: %s", cmd, expected)
	}
}

func TestBuildRunCommand_OpenCode_WithoutServerURL(t *testing.T) {
	p := GetRuntimeProfile("opencode")
	cmd := p.BuildRunCommand("/workspace", "my-task", "")
	expected := `/tools/opencode run --title my-task "$(cat /workspace/task.md)"`
	if cmd != expected {
		t.Errorf("unexpected command:\n  got:  %s\n  want: %s", cmd, expected)
	}
}

func TestBuildRunCommand_Crush_WithServerURL(t *testing.T) {
	p := GetRuntimeProfile("crush")
	cmd := p.BuildRunCommand("/workspace", "my-task", "http://server:4096")
	expected := `/tools/crush run --yolo "$(cat /workspace/task.md)"`
	if cmd != expected {
		t.Errorf("unexpected command:\n  got:  %s\n  want: %s", cmd, expected)
	}
}

func TestBuildRunCommand_Crush_WithoutServerURL(t *testing.T) {
	p := GetRuntimeProfile("crush")
	cmd := p.BuildRunCommand("/workspace", "my-task", "")
	expected := `/tools/crush run --yolo "$(cat /workspace/task.md)"`
	if cmd != expected {
		t.Errorf("unexpected command:\n  got:  %s\n  want: %s", cmd, expected)
	}
}

func TestBuildServeCommand_OpenCode(t *testing.T) {
	p := GetRuntimeProfile("opencode")
	cmd := p.BuildServeCommand(4096)
	expected := `/tools/opencode serve --port 4096 --hostname 0.0.0.0`
	if cmd != expected {
		t.Errorf("unexpected command:\n  got:  %s\n  want: %s", cmd, expected)
	}
}

func TestBuildServeCommand_Crush(t *testing.T) {
	p := GetRuntimeProfile("crush")
	cmd := p.BuildServeCommand(4096)
	expected := `/tools/crush server --host tcp://0.0.0.0:4096`
	if cmd != expected {
		t.Errorf("unexpected command:\n  got:  %s\n  want: %s", cmd, expected)
	}
}

func TestConfigPath_OpenCode(t *testing.T) {
	p := GetRuntimeProfile("opencode")
	if p.ConfigPath() != "/tools/opencode.json" {
		t.Errorf("unexpected config path: %q", p.ConfigPath())
	}
}

func TestConfigPath_Crush(t *testing.T) {
	p := GetRuntimeProfile("crush")
	if p.ConfigPath() != "/tools/crush.json" {
		t.Errorf("unexpected config path: %q", p.ConfigPath())
	}
}

func TestSymlinkCmd_OpenCode(t *testing.T) {
	p := GetRuntimeProfile("opencode")
	expected := "ln -sf /tools/opencode /usr/local/bin/opencode 2>/dev/null || true"
	if p.SymlinkCmd() != expected {
		t.Errorf("unexpected symlink cmd:\n  got:  %s\n  want: %s", p.SymlinkCmd(), expected)
	}
}

func TestSymlinkCmd_Crush(t *testing.T) {
	p := GetRuntimeProfile("crush")
	expected := "ln -sf /tools/crush /usr/local/bin/crush 2>/dev/null || true"
	if p.SymlinkCmd() != expected {
		t.Errorf("unexpected symlink cmd:\n  got:  %s\n  want: %s", p.SymlinkCmd(), expected)
	}
}

func TestSessionDBName_OpenCode(t *testing.T) {
	p := GetRuntimeProfile("opencode")
	if p.SessionDBName != "opencode.db" {
		t.Errorf("expected session DB name 'opencode.db', got %q", p.SessionDBName)
	}
}

func TestSessionDBName_Crush(t *testing.T) {
	p := GetRuntimeProfile("crush")
	if p.SessionDBName != "crush.db" {
		t.Errorf("expected session DB name 'crush.db', got %q", p.SessionDBName)
	}
}
