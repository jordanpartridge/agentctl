FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    curl git build-essential ca-certificates gnupg \
    python3 python3-pip python3-venv \
    php php-cli php-xml php-mbstring php-curl php-zip php-intl php-gd php-bcmath composer \
    php-dev php-pear \
    && pecl install pcov-1.0.12 \
    && echo "extension=pcov.so" > /etc/php/8.1/mods-available/pcov.ini \
    && ln -s /etc/php/8.1/mods-available/pcov.ini /etc/php/8.1/cli/conf.d/20-pcov.ini \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js 20
RUN mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_20.x nodistro main" | tee /etc/apt/sources.list.d/nodesource.list \
    && apt-get update && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install Go
RUN curl -fsSL https://go.dev/dl/go1.22.0.linux-amd64.tar.gz | tar -C /usr/local -xzf -
ENV PATH="/usr/local/go/bin:${PATH}"

# Install Rust
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/root/.cargo/bin:${PATH}"

# Install Claude CLI (kept for optional use) and opencode (the run-task harness)
RUN npm install -g @anthropic-ai/claude-code opencode-ai

# Worker tooling: shell linting, JSON wrangling, GitHub CLI
RUN apt-get update && apt-get install -y shellcheck jq \
    && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y gh \
    && rm -rf /var/lib/apt/lists/*

# Standard agentctl run-task entrypoint (opencode via mesh LLM router)
COPY scripts/run-task /usr/local/bin/run-task
# opencode provider config: mesh router, key from AGENT_LLM_KEY env (no baked secrets)
COPY scripts/opencode.json /opencode-config/opencode.json

# Create agent user
RUN useradd -m -s /bin/bash agent \
    && mkdir -p /home/agent/workspace \
    && chown -R agent:agent /home/agent

USER agent
WORKDIR /home/agent

# Install Rust for agent user
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/home/agent/.cargo/bin:${PATH}"

# Pre-warm package manager caches
# Composer: initialize global cache directory
RUN mkdir -p /home/agent/.cache/composer \
    && COMPOSER_CACHE_DIR=/home/agent/.cache/composer composer global require --no-interaction --no-plugins 2>/dev/null || true

# npm: initialize global cache directory
RUN mkdir -p /home/agent/.cache/npm \
    && npm config set cache /home/agent/.cache/npm

# Go: initialize module cache directory
RUN mkdir -p /home/agent/.cache/go-mod

# pip: initialize cache directory
RUN mkdir -p /home/agent/.cache/pip

# opencode: provider config + pre-installed SDK deps (runtime npm fetch inside
# workers is flaky and polluted the shared cache once already)
RUN mkdir -p /home/agent/.config/opencode \
    && cp /opencode-config/opencode.json /home/agent/.config/opencode/opencode.json \
    && cd /home/agent/.config/opencode \
    && npm install @ai-sdk/openai-compatible @opencode-ai/plugin

# Set environment variables for cache directories
ENV COMPOSER_CACHE_DIR="/home/agent/.cache/composer"
ENV npm_config_cache="/home/agent/.cache/npm"
ENV GOPATH="/home/agent/go"
ENV GOMODCACHE="/home/agent/.cache/go-mod"
ENV PIP_CACHE_DIR="/home/agent/.cache/pip"

CMD ["sleep", "infinity"]
