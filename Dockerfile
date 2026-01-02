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

# Install Claude CLI
RUN npm install -g @anthropic-ai/claude-code

# Create agent user
RUN useradd -m -s /bin/bash agent \
    && mkdir -p /home/agent/workspace \
    && chown -R agent:agent /home/agent

USER agent
WORKDIR /home/agent

# Install Rust for agent user
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
ENV PATH="/home/agent/.cargo/bin:${PATH}"

CMD ["sleep", "infinity"]
