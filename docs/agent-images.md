# Agent Images

KubeOpenCode provides **template agent images** that serve as starting points for building your own customized agents. These templates demonstrate the agent interface pattern and include common development tools, but are designed to be customized based on your specific requirements.

## Container Pattern

KubeOpenCode uses multiple container images:

1. **Init Container** (`agentImage`): Contains the runtime CLI (OpenCode or Crush), copies it to a shared `/tools` volume
2. **Worker Container** (`executorImage`): Your development environment that runs the runtime binary in Agent Deployments or ephemeral Task Pods
3. **Attach Container** (`attachImage`): Lightweight image (~25MB) used by Task Pods that connect to a running Agent

## Available Images

| Image | Type | Description |
|-------|------|-------------|
| `opencode` | Init Container | OpenCode CLI binary |
| `crush` | Init Container | Crush CLI binary |
| `devbox` | Worker (Executor) | Universal development environment with Go, Node, Python, kubectl, helm |
| `attach` | Attach Container | Lightweight image for connecting to Agent servers |
| `echo` | Testing | Minimal Alpine image for E2E testing |

## Image Resolution

When configuring an Agent, the controller resolves images as follows:

| Configuration | Init Container | Worker Container | Attach Container |
|--------------|----------------|------------------|------------------|
| Both `agentImage` and `executorImage` set | `agentImage` | `executorImage` | `attachImage` (default if unset) |
| Only `agentImage` set (legacy) | Default OpenCode image | `agentImage` | `attachImage` (default if unset) |
| Neither set | Default OpenCode image | Default devbox image | Default attach image |

### Default Images

Default images depend on the runtime selected:

**OpenCode (default):**
- Init: `ghcr.io/kubeopencode/kubeopencode-agent-opencode:latest`
- Executor: `ghcr.io/kubeopencode/kubeopencode-agent-devbox:latest`
- Attach: `ghcr.io/kubeopencode/kubeopencode-agent-attach:latest`

**Crush:**
- Init: `ghcr.io/kubeopencode/kubeopencode-agent-crush:latest`
- Executor: `ghcr.io/kubeopencode/kubeopencode-agent-devbox:latest`
- Attach: `ghcr.io/kubeopencode/kubeopencode-agent-attach:latest`

## Building Agent Images

### Local Development (Kind Clusters)

```bash
# Build OpenCode init container
make agent-build AGENT=opencode

# Build executor containers
make agent-build AGENT=devbox

# Customize registry and version
make agent-build AGENT=devbox IMG_REGISTRY=docker.io IMG_ORG=myorg VERSION=v1.0.0
```

### Remote/Production Clusters

For remote clusters (OpenShift, EKS, GKE, etc.), use multi-arch build and push:

```bash
# Multi-arch build and push
make agent-buildx AGENT=opencode
make agent-buildx AGENT=devbox
```

## Creating Custom Agent Images

For detailed guidance on building custom agent images, see the [Agent Developer Guide](../agents/README.md).

### Example Custom Executor

```dockerfile
FROM ghcr.io/kubeopencode/kubeopencode-agent-devbox:latest

# Add your custom tools
RUN apt-get update && apt-get install -y \
    your-custom-tool \
    another-tool

# Add custom configuration
COPY .bashrc /home/agent/.bashrc
```

## Next Steps

- [Getting Started](getting-started.md) - Installation and basic usage
- [Features](features.md) - Context system, concurrency, and more
- [Security](security.md) - RBAC, credential management, and best practices
- [Agent Developer Guide](../agents/README.md) - Detailed agent development
