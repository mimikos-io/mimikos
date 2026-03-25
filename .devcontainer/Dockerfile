# Development Container for Mimikos
# Matches CI environment: Go 1.25.0, golangci-lint v2.4.0

FROM golang:1.25.0-bookworm AS base

# Install system dependencies
RUN apt-get update && apt-get install -y \
    curl \
    git \
    make \
    ca-certificates \
    python3 \
    python3-pip \
    vim \
    nano \
    && rm -rf /var/lib/apt/lists/*

# Install pre-commit (override PEP 668 for container environment)
RUN pip3 install --no-cache-dir --break-system-packages pre-commit

# Install golangci-lint v2.4.0 (matching CI version)
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
    sh -s -- -b $(go env GOPATH)/bin v2.4.0

# Create non-root user
RUN groupadd --gid 1000 mimikos && \
    useradd --uid 1000 --gid mimikos --shell /bin/bash --create-home mimikos

# Set up workspace
WORKDIR /workspace
RUN chown mimikos:mimikos /workspace

# Set Go environment
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOPROXY=https://proxy.golang.org,direct \
    GOSUMDB=sum.golang.org

# Configure Git with a default editor
RUN git config --system core.editor nano

# Ensure Go tools are in PATH
ENV PATH="${PATH}:/home/mimikos/go/bin"

USER mimikos
CMD ["sleep", "infinity"]
