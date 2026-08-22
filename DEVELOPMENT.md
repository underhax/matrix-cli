# matrix-cli Development Guide

This guide details the prerequisites, local workflow, testing standards, and build instructions for contributors and developers working on `matrix-cli`.

---

## Prerequisites

Ensure the following dependencies are installed in your local environment:

- **Git**
- **Go 1.27+**
- **C Compiler (GCC / Clang)** (required for CGO / SQLite3 support)
- **GNU Make**
- **golangci-lint**: [https://golangci-lint.run/docs/welcome/install/](https://golangci-lint.run/docs/welcome/install/)
- **govulncheck**: `go install golang.org/x/vuln/cmd/govulncheck@latest`
- **trivy**: [https://trivy.dev/docs/latest/getting-started/installation/](https://trivy.dev/docs/latest/getting-started/installation/)
- **hadolint**: [https://github.com/hadolint/hadolint#install](https://github.com/hadolint/hadolint#install)
- **shellcheck**: [https://github.com/koalaman/shellcheck#installing](https://github.com/koalaman/shellcheck#installing)

---

## Repository Initialization

1. **Clone the repository:**
   ```bash
   git clone https://github.com/underhax/matrix-cli.git
   cd matrix-cli
   ```

2. **Download dependencies:**
   ```bash
   go mod download
   ```

---

## Code Quality & Testing Workflow

All validation, testing, and linting commands are accessible via both the `Makefile` and direct standard Go toolchain commands.

### 1. Code Formatting, Linting & Compilation Check
Validate code formatting, run strict `golangci-lint` rules, and verify that all packages compile with pure-Go Olm tags (`-tags goolm`):
```bash
# Using Make:
make check

# Direct Go commands (entire project or target a specific package):
gofmt -s -w .
golangci-lint run ./...
go build -tags goolm ./...

# Target a specific module:
golangci-lint run ./<module_name>/...
go build -tags goolm ./<module_name>/...
```

### 2. Unit Testing
Execute the test suite with verbose output (`-v`), race condition detection (`-race`), and pure-Go Olm ratchets (`-tags goolm`):
```bash
# Using Make:
make test

# Direct Go commands:
go test -tags goolm -v -race ./...

# Target a specific module or run an individual test function:
go test -tags goolm -v -race ./<module_name>/...
go test -tags goolm -v -race -run <TestName> ./<module_name>/...
```

### 3. Code Coverage
Run tests and generate statement-level coverage reports per function:
```bash
# Using Make:
make coverage

# Direct Go commands:
go test -tags goolm -v -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Target a specific module:
go test -tags goolm -v -race -coverprofile=coverage.out ./<module_name>/...
go tool cover -func=coverage.out

# View interactive coverage heatmap in browser:
go tool cover -html=coverage.out
```

### 4. Vulnerability & Security Scanning
Scan dependencies and the filesystem for known security advisories:
```bash
# Using Make:
make vulncheck
make trivy

# Direct commands:
govulncheck -tags goolm ./...
trivy fs --severity CRITICAL,HIGH .
```

### 5. Dockerfile, Entrypoint & Compose Validation
Verify that `docker/Dockerfile`, `docker/entrypoint.sh`, and Compose definitions pass all checks:
```bash
# Using Make:
make docker

# Direct commands:
hadolint docker/Dockerfile
shellcheck docker/entrypoint.sh
```
If you have Docker Compose installed locally, you can also validate the syntax of your compose files:
```bash
docker compose -f docker/docker-compose.yaml config -q
docker compose -f docker/docker-compose.dev.yaml config -q
```

### 6. Full Pre-flight Verification
Before submitting a pull request, run the complete verification suite:
```bash
make verify
```

---

## Build Workflow

### 1. Standard Local Build
Compile the `matrix-cli` binary with pure-Go Olm ratchets (`goolm`) and trimmed file paths:
```bash
make build
```
The compiled executable will be placed in the repository root as `./matrix-cli`.

### 2. Custom Version Injection
Inject a specific release version tag into the compiled binary via the `VERSION` variable:
```bash
make build VERSION=v0.1.0
```

### 3. Direct Go Build Command
If compiling manually without `make`:
```bash
CGO_ENABLED=1 go build -trimpath -tags goolm -ldflags "-s -w -X main.AppVersion=v0.1.0" -o matrix-cli ./cmd/matrix-cli
```

### 4. Cross-Compilation (CGO & Toolchain Requirements)
Because `matrix-cli` utilizes SQLite3 for session and cryptographic state persistence, **CGO is required (`CGO_ENABLED=1`)**. When cross-compiling for target architectures other than your host system, you must specify the corresponding C cross-compiler via the `CC` environment variable.

#### Linux Targets (Static Linking with musl-cross)
Using `musl-cross` toolchains (e.g., from [musl.cc](https://musl.cc)) to produce statically linked, portable Linux binaries:

```bash
# Linux x86_64 (amd64):
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc \
  go build -tags goolm -trimpath -ldflags "-s -w -X main.AppVersion=v0.1.0 -extldflags \"-static\"" -o matrix-cli ./cmd/matrix-cli

# Linux ARM64 (aarch64):
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-musl-gcc \
  go build -tags goolm -trimpath -ldflags "-s -w -X main.AppVersion=v0.1.0 -extldflags \"-static\"" -o matrix-cli ./cmd/matrix-cli

# Linux ARMv7:
CGO_ENABLED=1 GOOS=linux GOARCH=arm GOARM=7 CC=arm-linux-musleabihf-gcc \
  go build -tags goolm -trimpath -ldflags "-s -w -X main.AppVersion=v0.1.0 -extldflags \"-static\"" -o matrix-cli ./cmd/matrix-cli

# Linux MIPSLE (Routers / Keenetic / OpenWrt):
CGO_ENABLED=1 GOOS=linux GOARCH=mipsle GOMIPS=softfloat CC=mipsel-linux-muslsf-gcc \
  go build -tags goolm -trimpath -ldflags "-s -w -X main.AppVersion=v0.1.0 -extldflags \"-static\"" -o matrix-cli ./cmd/matrix-cli

# Linux MIPS (Big-Endian):
CGO_ENABLED=1 GOOS=linux GOARCH=mips GOMIPS=softfloat CC=mips-linux-muslsf-gcc \
  go build -tags goolm -trimpath -ldflags "-s -w -X main.AppVersion=v0.1.0 -extldflags \"-static\"" -o matrix-cli ./cmd/matrix-cli
```

#### Windows Targets (MinGW-w64)
Requires the `mingw-w64` or `llvm-mingw` toolchain installed (`brew install mingw-w64` on macOS or `apt-get install gcc-mingw-w64` on Linux):

```bash
# Windows x64 (amd64):
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
  go build -tags goolm -trimpath -ldflags "-s -w -X main.AppVersion=v0.1.0" -o matrix-cli.exe ./cmd/matrix-cli

# Windows ARM64:
CGO_ENABLED=1 GOOS=windows GOARCH=arm64 CC=aarch64-w64-mingw32-gcc \
  go build -tags goolm -trimpath -ldflags "-s -w -X main.AppVersion=v0.1.0" -o matrix-cli.exe ./cmd/matrix-cli
```

#### macOS Targets (Apple Silicon & Intel)
- **On macOS hosts:** The built-in `clang` compiler supports both architectures out of the box:
  ```bash
  # Apple Silicon (arm64):
  CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
    go build -tags goolm -trimpath -ldflags "-s -w -X main.AppVersion=v0.1.0" -o matrix-cli ./cmd/matrix-cli

  # Intel x86_64 (amd64):
  CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
    go build -tags goolm -trimpath -ldflags "-s -w -X main.AppVersion=v0.1.0" -o matrix-cli ./cmd/matrix-cli
  ```
- **On non-macOS hosts (Linux / Windows):** Cross-compiling Darwin binaries with CGO requires a Darwin SDK cross-toolchain (e.g., `osxcross` with `CC=<target>-apple-darwin-clang`).

---

## Docker Build & Deployment

### 1. Build the Docker image locally
Build the multi-stage container image natively for your local architecture:
```bash
docker build -t matrix-cli:dev --build-arg APP_VERSION=dev -f docker/Dockerfile .
```

To cross-compile for another architecture on a modern Docker engine with BuildKit:
```bash
# For ARM64:
docker build --platform linux/arm64 -t matrix-cli:dev --build-arg APP_VERSION=dev -f docker/Dockerfile .

# For AMD64:
docker build --platform linux/amd64 -t matrix-cli:dev --build-arg APP_VERSION=dev -f docker/Dockerfile .
```

### 2. Build and run using Docker Compose
You can utilize the provided development compose file to automatically build and start the container with development settings applied:
```bash
# Build natively for your local architecture:
docker compose -f docker/docker-compose.dev.yaml build

# Start the container:
docker compose -f docker/docker-compose.dev.yaml up
```

To cross-compile for another architecture using Docker Compose:
```bash
# For ARM64:
DOCKER_DEFAULT_PLATFORM=linux/arm64 docker compose -f docker/docker-compose.dev.yaml build

# For AMD64:
DOCKER_DEFAULT_PLATFORM=linux/amd64 docker compose -f docker/docker-compose.dev.yaml build
```

---

## Makefile Targets Reference

| Target | Description |
| --- | --- |
| `make check` | Verifies code formatting with `gofmt`, runs `golangci-lint`, and checks package compilation. |
| `make test` | Runs the test suite with verbose output (`-v`), race detection (`-race`), and `-tags goolm`. |
| `make coverage` | Runs tests, produces `coverage.out`, and prints statement coverage per function. |
| `make vulncheck` | Analyzes dependencies for known vulnerabilities using `govulncheck`. |
| `make docker` | Lints `docker/Dockerfile` using `hadolint`. |
| `make trivy` | Performs a filesystem security scan for `CRITICAL` and `HIGH` vulnerabilities. |
| `make verify` | Runs the entire pre-submission verification pipeline (`check`, `test`, `vulncheck`, `trivy`, `docker`). |
| `make build` | Compiles the production binary with `-trimpath`, `-tags goolm`, and version injection. |
