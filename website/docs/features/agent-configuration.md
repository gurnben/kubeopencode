# Agent Configuration

Agent centralizes execution environment configuration:

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: Agent
metadata:
  name: default
spec:
  profile: "Default development agent with org standards and GitHub access"
  agentImage: ghcr.io/kubeopencode/kubeopencode-agent-opencode:latest
  executorImage: ghcr.io/kubeopencode/kubeopencode-agent-devbox:latest
  workspaceDir: /workspace
  serviceAccountName: kubeopencode-agent

  # Default contexts for all tasks (inline ContextItems)
  contexts:
    - type: Text
      text: |
        # Organization Standards
        - Use signed commits
        - Follow Go conventions

  # Credentials (secrets as env vars or file mounts)
  credentials:
    - name: github-token
      secretRef:
        name: github-creds
        key: token
      env: GITHUB_TOKEN

    - name: ssh-key
      secretRef:
        name: ssh-keys
        key: id_rsa
      mountPath: /home/agent/.ssh/id_rsa
      fileMode: 0400
```

## Runtime Configuration

The `config` field provides runtime-specific configuration as an inline JSON string. The format depends on which runtime you use.

### OpenCode Configuration

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: Agent
metadata:
  name: opencode-agent
spec:
  profile: "OpenCode agent with custom model configuration"
  agentImage: ghcr.io/kubeopencode/kubeopencode-agent-opencode:latest
  executorImage: ghcr.io/kubeopencode/kubeopencode-agent-devbox:latest
  workspaceDir: /workspace
  serviceAccountName: kubeopencode-agent
  config: |
    {
      "$schema": "https://opencode.ai/config.json",
      "model": "google/gemini-2.5-pro",
      "small_model": "google/gemini-2.5-flash"
    }
```

The configuration is written to `/tools/opencode.json` and the `OPENCODE_CONFIG` environment variable is set automatically. See [OpenCode configuration schema](https://opencode.ai/config.json) for available options.

### Crush Configuration

Crush uses a different config format. Models are selected via the `models` map with `large` and `small` entries, each specifying a `model` ID and `provider`:

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: Agent
metadata:
  name: crush-agent
spec:
  runtime: crush
  profile: "Crush agent with Claude on Vertex AI"
  agentImage: ghcr.io/kubeopencode/kubeopencode-agent-crush:latest
  executorImage: ghcr.io/kubeopencode/kubeopencode-agent-devbox:latest
  workspaceDir: /workspace
  serviceAccountName: kubeopencode-agent
  config: |
    {
      "models": {
        "large": {
          "model": "claude-sonnet-4-6",
          "provider": "vertexai"
        },
        "small": {
          "model": "claude-haiku-4-5-20251001",
          "provider": "vertexai"
        }
      }
    }
```

The configuration is written to the Crush data directory and discovered via the `CRUSH_GLOBAL_DATA` environment variable, which is set automatically. Use `crush models` inside the agent terminal to list available model IDs for each provider.

:::tip Vertex AI Credentials
When using Vertex AI as a provider, you must also provide GCP Application Default Credentials and the `VERTEXAI_PROJECT` / `VERTEXAI_LOCATION` environment variables via the `credentials` field. See [Runtime Selection — Vertex AI Example](#vertex-ai-example) below.
:::

## Runtime Selection

KubeOpenCode supports multiple coding agent runtimes. Set the `runtime` field
on your Agent or AgentTemplate to choose which runtime to use:

| Runtime | Binary | Config Format | Description |
|---------|--------|---------------|-------------|
| `opencode` (default) | OpenCode | `"model": "provider/model-id"` | Full-featured coding agent by Anomaly |
| `crush` | Crush | `"models": {"large": {...}}` | Agentic coding tool by Charmbracelet |

### Per-Agent

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: Agent
metadata:
  name: my-crush-agent
spec:
  runtime: crush
  agentImage: ghcr.io/kubeopencode/kubeopencode-agent-crush:latest
  executorImage: ghcr.io/kubeopencode/kubeopencode-agent-devbox:latest
  config: |
    {
      "models": {
        "large": {
          "model": "claude-sonnet-4-6",
          "provider": "vertexai"
        }
      }
    }
```

### Cluster-Wide Default

Set a default runtime for all Agents via KubeOpenCodeConfig:

```yaml
apiVersion: kubeopencode.io/v1alpha1
kind: KubeOpenCodeConfig
metadata:
  name: cluster
spec:
  defaultRuntime: crush
```

Agents that explicitly set `runtime` override the cluster default.

### Vertex AI Example

To use Claude on Google Vertex AI with the Crush runtime, you need three things:

1. A GCP service account key (JSON) with the **Vertex AI User** role
2. The `VERTEXAI_PROJECT` and `VERTEXAI_LOCATION` environment variables
3. The ADC file mounted where the GCP SDK can discover it

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: gcp-vertex-config
type: Opaque
stringData:
  VERTEXAI_PROJECT: "my-gcp-project"
  VERTEXAI_LOCATION: "us-east5"
---
apiVersion: v1
kind: Secret
metadata:
  name: gcp-adc
type: Opaque
data:
  application_default_credentials.json: <base64-encoded service account key JSON>
---
apiVersion: kubeopencode.io/v1alpha1
kind: AgentTemplate
metadata:
  name: crush-vertex
spec:
  runtime: crush
  agentImage: ghcr.io/kubeopencode/kubeopencode-agent-crush:latest
  executorImage: ghcr.io/kubeopencode/kubeopencode-agent-devbox:latest
  workspaceDir: /workspace
  config: |
    {
      "models": {
        "large": {
          "model": "claude-sonnet-4-6",
          "provider": "vertexai"
        },
        "small": {
          "model": "claude-haiku-4-5-20251001",
          "provider": "vertexai"
        }
      }
    }
  credentials:
    - name: gcloud-adc
      secretRef:
        name: gcp-adc
        key: application_default_credentials.json
      mountPath: /tmp/.config/gcloud/application_default_credentials.json
    - name: vertex-project
      secretRef:
        name: gcp-vertex-config
        key: VERTEXAI_PROJECT
      env: VERTEXAI_PROJECT
    - name: vertex-location
      secretRef:
        name: gcp-vertex-config
        key: VERTEXAI_LOCATION
      env: VERTEXAI_LOCATION
```

The GCP SDK discovers the ADC file automatically at `$HOME/.config/gcloud/application_default_credentials.json` (where `HOME=/tmp` in agent pods). No `gcloud` CLI is required — the Go SDK reads the service account JSON directly.
