# agentctl

Container-based Claude Code agent orchestrator. Spawn isolated Claude agents in Podman containers that work on tasks until they're complete.

## Features

- **Isolated Environments**: Each agent runs in its own container with a cloned repo
- **Auto-auth**: Copies your Claude credentials into containers automatically
- **Ralph Wiggum Mode**: Keeps running Claude until tests pass and changes are committed
- **Multi-language Support**: Detects and runs tests for Go, Node.js, PHP (Pest), Python, and Rust

## Requirements

- [Podman](https://podman.io/) installed and running
- [Claude Code CLI](https://github.com/anthropics/claude-code) installed
- `agent-devbox:latest` container image (see below)
- GitHub CLI (`gh`) for token authentication

## Installation

```bash
go install github.com/jordanpartridge/agentctl/cmd/agentctl@latest
```

Or build from source:
```bash
git clone https://github.com/jordanpartridge/agentctl
cd agentctl
go build -o agentctl ./cmd/agentctl
sudo mv agentctl /usr/local/bin/
```

## Usage

### Spawn an agent
```bash
agentctl spawn my-agent https://github.com/user/repo main
```

### Run a task until complete (Ralph Wiggum mode)
```bash
agentctl run my-agent "Fix the failing tests in src/auth.go" 5
```

This will:
1. Run Claude with the task
2. Check if tests pass and changes are committed
3. If not, re-run Claude with context about what's still needed
4. Repeat until done or max attempts reached

### Check agent status
```bash
agentctl check my-agent
```

### List all agents
```bash
agentctl list
```

### View Claude logs
```bash
agentctl logs my-agent
```

### Shell into container
```bash
agentctl shell my-agent
```

### Kill an agent
```bash
agentctl kill my-agent
```

## Building the agent-devbox Image

Create a `Dockerfile`:

```dockerfile
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y \
    curl git build-essential \
    nodejs npm \
    golang-go \
    php php-cli composer \
    python3 python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Install Claude CLI
RUN npm install -g @anthropic-ai/claude-code

# Create agent user
RUN useradd -m -s /bin/bash agent
USER agent
WORKDIR /home/agent

RUN mkdir -p /home/agent/workspace

CMD ["sleep", "infinity"]
```

Build it:
```bash
podman build -t agent-devbox:latest .
```

## How It Works

1. **Spawn** creates a container, copies Claude auth, and clones the repo
2. **Run** executes Claude with `--dangerously-skip-permissions` in a loop
3. After each Claude run, it checks:
   - Do tests pass? (auto-detects test runner)
   - Are there uncommitted changes?
4. If tests fail or changes exist, re-prompts Claude with status
5. Continues until success or max attempts

## License

MIT
