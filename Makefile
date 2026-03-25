.PHONY: start run check fix build reset help

# Variables
BINARY_NAME=mimikos
GO_VERSION=1.25
DEV_CONTAINER_NAME=mimikos-dev-container

#===============================================================================
# HELPERS
#===============================================================================

# Helper function to ensure we're not inside dev container (for host-only commands)
ensure-not-in-dev-container:
	@if [ -f /.dockerenv ] && [ "$$PWD" = "/workspace" ]; then \
		echo "❌ This command should not run inside dev container"; \
		echo ""; \
		echo "💡 You're currently inside the dev container!"; \
		echo "🏠 Please run this command from the host machine:"; \
		echo "   exit              # Exit dev container"; \
		echo "   make $(firstword $(MAKECMDGOALS))     # Run command on host"; \
		echo ""; \
		echo "💡 Use the dev container for coding and testing."; \
		echo "💡 Use the host for environment management."; \
		exit 1; \
	fi

# Helper function to validate Docker environment
check-docker-environment:
	@echo "🔍 Validating Docker environment..."
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "❌ Docker not found. Please install Docker first:"; \
		echo "   - macOS: Docker Desktop from docker.com"; \
		echo "   - Ubuntu: sudo apt install docker.io"; \
		echo "   - Windows: Docker Desktop from docker.com"; \
		exit 1; \
	fi
	@if ! docker info >/dev/null 2>&1; then \
		echo "❌ Docker daemon not running"; \
		echo "💡 Please start Docker Desktop or Docker service"; \
		exit 1; \
	fi
	@echo "✅ Docker environment validated"

# Helper function to check and install devcontainer CLI
check-devcontainer-cli:
	@echo "🔍 Checking devcontainer CLI availability..."
	@if ! command -v devcontainer >/dev/null 2>&1; then \
		echo "📦 devcontainer CLI not found, installing..."; \
		if ! command -v npm >/dev/null 2>&1; then \
			echo "❌ npm not found. Please install Node.js first:"; \
			echo "   - macOS: brew install node"; \
			echo "   - Ubuntu: sudo apt install nodejs npm"; \
			echo "   - Windows: Download from https://nodejs.org"; \
			exit 1; \
		fi; \
		echo "⏳ Installing @devcontainers/cli globally..."; \
		if npm install -g @devcontainers/cli; then \
			echo "✅ devcontainer CLI installed successfully"; \
		else \
			echo "❌ Failed to install devcontainer CLI"; \
			echo "💡 You may need to run with sudo or check npm permissions"; \
			echo "💡 Alternative: npm install -g @devcontainers/cli --unsafe-perm=true"; \
			exit 1; \
		fi; \
	else \
		echo "✅ devcontainer CLI is available"; \
	fi

# Helper function to check environment state
check-environment-state:
	@echo "🔍 Checking development environment state..."
	@$(eval DEV_CONTAINER_RUNNING := $(shell docker ps --format "table {{.Names}}" | grep -q "$(DEV_CONTAINER_NAME)" 2>/dev/null && echo "yes" || echo "no"))
	@$(eval DEV_CONTAINER_EXISTS := $(shell docker ps -a --format "table {{.Names}}" | grep -q "$(DEV_CONTAINER_NAME)" 2>/dev/null && echo "yes" || echo "no"))

# Helper function to check dev container state (for initial setup)
check-dev-container:
	@echo "🏗️ Checking dev container state..."
	@if [ -f .devcontainer/devcontainer.json ]; then \
		echo "📋 Dev container configuration found"; \
		if devcontainer read-configuration --workspace-folder . >/dev/null 2>&1; then \
			echo "✅ Dev container configuration valid"; \
			if docker ps --format "table {{.Names}}" | grep -q "$(DEV_CONTAINER_NAME)" 2>/dev/null; then \
				echo "🏃 Dev container already running"; \
			else \
				echo "💤 Dev container exists but not running"; \
				echo "🏗️ Building/starting dev container..."; \
				if devcontainer up --workspace-folder .; then \
					echo "✅ Dev container ready"; \
				else \
					echo "⚠️ Dev container build failed, continuing with host-based development"; \
				fi; \
			fi; \
		else \
			echo "🏗️ Building dev container from configuration..."; \
			if devcontainer up --workspace-folder .; then \
				echo "✅ Dev container built and ready"; \
			else \
				echo "⚠️ Dev container build failed, continuing with host-based development"; \
			fi; \
		fi; \
	else \
		echo "⚠️ No dev container configuration found at .devcontainer/devcontainer.json"; \
	fi

# Helper function to restart existing dev container
restart-dev-container:
	@echo "🔄 Restarting dev container..."
	@if devcontainer up --workspace-folder . >/dev/null 2>&1; then \
		echo "✅ Dev container restarted"; \
	else \
		echo "⚠️ Dev container restart failed, trying full rebuild..."; \
		$(MAKE) start-full-setup; \
	fi

# Helper function to exec into dev container
exec-dev-container:
	@echo "🏃 Entering development container..."
	@if docker ps --format "table {{.Names}}" | grep -q "$(DEV_CONTAINER_NAME)" 2>/dev/null; then \
		echo "🐳 Executing into running dev container..."; \
		docker exec -it $(DEV_CONTAINER_NAME) /bin/bash; \
	else \
		echo "⚠️ Dev container not running, starting it first..."; \
		if devcontainer up --workspace-folder . >/dev/null 2>&1; then \
			echo "✅ Dev container started"; \
			echo "🐳 Executing into dev container..."; \
			docker exec -it $(DEV_CONTAINER_NAME) /bin/bash; \
		else \
			echo "❌ Failed to start dev container"; \
			echo "💡 Try: make reset && make start"; \
			exit 1; \
		fi; \
	fi

# Helper function to get version info
define get-version-info
	$(eval VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev"))
	$(eval COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown"))
	$(eval BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S UTC'))
endef

#===============================================================================
# GETTING STARTED
#===============================================================================

# Begin working (intelligent setup + exec into dev container)
start: ensure-not-in-dev-container check-docker-environment check-devcontainer-cli check-environment-state
	@if [ "$(DEV_CONTAINER_RUNNING)" = "yes" ]; then \
		echo "🎉 Development environment is already running!"; \
		$(MAKE) exec-dev-container; \
	elif [ "$(DEV_CONTAINER_EXISTS)" = "yes" ]; then \
		echo "🔄 Development environment exists but is stopped"; \
		echo "⚡ Restarting dev container..."; \
		$(MAKE) restart-dev-container; \
		echo ""; \
		$(MAKE) exec-dev-container; \
	else \
		echo "🚀 Setting up complete development environment from scratch..."; \
		$(MAKE) start-full-setup; \
		echo ""; \
		$(MAKE) exec-dev-container; \
	fi

# Internal target for full environment setup
start-full-setup: check-dev-container
	@echo ""
	@echo "📦 Downloading dependencies..."
	go mod download
	go mod verify
	@echo "✅ Go dependencies ready"
	@echo ""
	@echo "🎉 Development environment ready!"
	@echo ""
	@echo "📋 Environment Status:"
	@echo "  ✅ Dev container:     Built and configured"
	@echo "  ✅ Dependencies:      Downloaded and verified"

#===============================================================================
# DAILY DEVELOPMENT
#===============================================================================

# Execute something (run, run test, run test unit, run benchmark, run linter)
run:
	@if [ "$(filter-out $@,$(MAKECMDGOALS))" = "" ]; then \
		echo "🏃 Starting mimikos mock server..."; \
		go run ./cmd/mimikos; \
	elif [ "$(filter-out $@,$(MAKECMDGOALS))" = "test" ]; then \
		$(MAKE) run-test; \
	elif [ "$(filter-out $@,$(MAKECMDGOALS))" = "test unit" ]; then \
		$(MAKE) run-test-unit; \
	elif [ "$(filter-out $@,$(MAKECMDGOALS))" = "test integration" ]; then \
		$(MAKE) run-test-integration; \
	elif [ "$(filter-out $@,$(MAKECMDGOALS))" = "test race" ]; then \
		$(MAKE) run-test-race; \
	elif [ "$(filter-out $@,$(MAKECMDGOALS))" = "benchmark" ]; then \
		$(MAKE) run-benchmark; \
	elif [ "$(filter-out $@,$(MAKECMDGOALS))" = "linter" ]; then \
		$(MAKE) run-linter; \
	else \
		echo "❌ Unknown run command: $(filter-out $@,$(MAKECMDGOALS))"; \
		echo "📖 Available run commands:"; \
		echo "  make run                    # Start mimikos mock server"; \
		echo "  make run test               # Run all tests"; \
		echo "  make run test unit          # Run unit tests only"; \
		echo "  make run test integration   # Run integration tests"; \
		echo "  make run test race          # Run tests with race detection"; \
		echo "  make run benchmark          # Run benchmark tests"; \
		echo "  make run linter             # Run linter"; \
		exit 1; \
	fi

# Internal run targets
run-test:
	@echo "🧪 Running all tests..."
	go test -short -v -cover ./... && go test -v -cover ./...

run-test-unit:
	@echo "🧪 Running unit tests..."
	go test -short -v -cover ./...

run-test-integration:
	@echo "🧪 Running integration tests..."
	go test -v -cover -timeout=10m ./...

run-test-race:
	@echo "🧪 Running tests with race detection..."
	go test -v -race -cover ./...

run-benchmark:
	@echo "⚡ Running benchmark tests..."
	go test -v -bench=. -benchmem -run='^$$' ./...

run-linter:
	@echo "📝 Running linter..."
	golangci-lint run

# Verify code quality (lint + test + vet)
check:
	@echo "🔍 Checking code quality..."
	@echo "📝 Running linter..."
	golangci-lint run
	@echo "🧪 Running unit tests..."
	go test -short -v -cover ./...
	@echo "🧪 Running integration tests..."
	go test -v -cover -timeout=10m ./...
	@echo "🔬 Running vet..."
	go vet ./...
	@echo "✅ All checks passed!"

# Repair issues (format + tidy + clean artifacts)
fix:
	@echo "🔧 Auto-fixing issues..."
	@echo "📝 Formatting code..."
	golangci-lint fmt
	@echo "🧹 Tidying dependencies..."
	go mod tidy
	@echo "🗑️ Cleaning build artifacts..."
	go clean
	rm -rf bin/ build/
	@echo "✅ Auto-fix complete!"

#===============================================================================
# BUILD
#===============================================================================

# Create artifacts (build, build prod)
build:
	@if [ "$(filter-out $@,$(MAKECMDGOALS))" = "prod" ]; then \
		$(MAKE) build-prod; \
	elif [ "$(filter-out $@,$(MAKECMDGOALS))" = "" ]; then \
		$(MAKE) build-dev; \
	else \
		echo "❌ Unknown build target: $(filter-out $@,$(MAKECMDGOALS))"; \
		echo "Available targets:"; \
		echo "  make build           # Development build"; \
		echo "  make build prod      # Production build (static, linux)"; \
		exit 1; \
	fi

build-dev:
	@echo "🔨 Building development binary..."
	$(call get-version-info)
	go build -ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(COMMIT) -X 'main.buildTime=$(BUILD_TIME)'" -o bin/$(BINARY_NAME) ./cmd/mimikos
	@echo "✅ Built: bin/$(BINARY_NAME)"

# Internal build targets
build-prod:
	@echo "🔨 Building production binary..."
	$(call get-version-info)
	@echo "📦 Building $(BINARY_NAME)..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
		-ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(COMMIT) -X 'main.buildTime=$(BUILD_TIME)'" \
		-o build/$(BINARY_NAME) ./cmd/mimikos
	@echo "✅ Production build complete: build/$(BINARY_NAME)"

#===============================================================================
# MAINTENANCE
#===============================================================================

# Start fresh (clean everything + stop dev container)
reset: ensure-not-in-dev-container
	@echo "🔄 Performing reset..."
	@echo "🐳 Stopping dev container..."
	-@if docker ps --format "table {{.Names}}" | grep -q "$(DEV_CONTAINER_NAME)" 2>/dev/null; then \
		docker stop $(DEV_CONTAINER_NAME); \
		echo "✅ Dev container stopped"; \
	fi
	-@if docker ps -a --format "table {{.Names}}" | grep -q "$(DEV_CONTAINER_NAME)" 2>/dev/null; then \
		docker rm $(DEV_CONTAINER_NAME); \
		echo "✅ Dev container removed"; \
	fi
	@echo "🗑️ Cleaning build artifacts..."
	go clean
	rm -rf bin/ build/
	@echo "🧹 Tidying dependencies..."
	go mod tidy
	@echo ""
	@echo "💥 Reset complete!"
	@echo ""
	@echo "🚀 To rebuild everything fresh:"
	@echo "   make start    # Complete environment setup"
	@echo ""
	@echo "📋 What was cleaned:"
	@echo "  ✅ Dev container stopped and removed"
	@echo "  ✅ Build artifacts removed"
	@echo "  ✅ Dependencies tidied"

#===============================================================================
# HELP
#===============================================================================

help:
	@echo "***************************************************************"
	@echo "*              🎭 Mimikos Development                         *"
	@echo "***************************************************************"
	@echo ""
	@echo "🚀 Getting Started:"
	@echo "    start   - Begin working (smart setup + exec into dev container)"
	@echo "    run     - Execute something (run, run test, run linter)"
	@echo ""
	@echo "🛠️  Daily Development:"
	@echo "    check   - Verify code quality (lint + test + vet)"
	@echo "    fix     - Repair issues (format + tidy + clean artifacts)"
	@echo ""
	@echo "🏗️  Build:"
	@echo "    build      - Build development binary"
	@echo "    build prod - Build production binary (static, linux)"
	@echo ""
	@echo "🔧 Maintenance:"
	@echo "    reset   - Start fresh (stop dev container + clean artifacts)"
	@echo ""
	@echo "📖 Examples:"
	@echo "    🚀 Getting Started:"
	@echo "        make start                    # Smart setup + enter dev container"
	@echo "        make run                      # Start mimikos mock server"
	@echo ""
	@echo "    🧪 Testing:"
	@echo "        make run test                 # Run all tests"
	@echo "        make run test unit            # Run unit tests only"
	@echo "        make run test integration     # Run integration tests"
	@echo "        make run test race            # Run tests with race detection"
	@echo "        make run benchmark            # Run benchmark tests"
	@echo "        make run linter               # Run linter"
	@echo "        make check                    # Check code quality before commit"
	@echo ""
	@echo "    🏗️  Build:"
	@echo "        make build                    # Development build"
	@echo "        make build prod               # Production build (static, linux)"
	@echo ""
	@echo "    🆘 Troubleshooting:"
	@echo "        make reset                    # Clean slate (when things go wrong)"
	@echo ""
	@echo "⚡ Quick Start:"
	@echo "    🆕 New to this project?          make start"
	@echo "    💻 Daily development?            make start"
	@echo "    🚀 Ready to deploy?              make check && make build prod"
	@echo ""

# Handle command line arguments for parameterized commands
# These are pseudo-targets that act as arguments to run/build commands
# They must be declared as .PHONY and have empty recipes to prevent Make errors
.PHONY: test unit integration race benchmark linter prod

test unit integration race benchmark linter prod:
	@:
