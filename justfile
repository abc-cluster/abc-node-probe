set shell := ["bash", "-euo", "pipefail", "-c"]

# abc-node-probe — routine dev tasks (`just` / `just --list`).
export CGO_ENABLED := "0"

bin := "abc-node-probe"

# Show recipes (default).
default:
    @just --list

# Fast dev binary at ./abc-node-probe (no injected version).
build:
    go build -trimpath -o ./{{ bin }} .

# Release-style binary with git-derived version / build time / commit (same -X wiring as former Makefile `build`).
build-release out="./abc-node-probe":
    #!/usr/bin/env bash
    set -euo pipefail
    VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
    BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
    GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
    mkdir -p "$(dirname "{{ out }}")"
    go build -trimpath \
      -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}'" \
      -o "{{ out }}" .

# Install release-style binary to ~/bin/abc-node-probe (same -X wiring as build-release).
install-local:
    #!/usr/bin/env bash
    set -euo pipefail
    VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
    BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
    GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
    mkdir -p "${HOME}/bin"
    tmp="${HOME}/bin/{{ bin }}.just.tmp.$$"
    go build -trimpath \
      -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}'" \
      -o "${tmp}" .
    mv "${tmp}" "${HOME}/bin/{{ bin }}"
    chmod 0755 "${HOME}/bin/{{ bin }}"
    echo "Installed ${HOME}/bin/{{ bin }}"

# Cross-compile one target into dist/ (set GOOS/GOARCH; defaults to current platform).
dist:
    #!/usr/bin/env bash
    set -euo pipefail
    GOOS="${GOOS:-$(go env GOOS)}"
    GOARCH="${GOARCH:-$(go env GOARCH)}"
    VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
    BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
    GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
    mkdir -p dist
    EXT=""
    if [[ "${GOOS}" == "windows" ]]; then EXT=".exe"; fi
    OUT="dist/{{ bin }}-${GOOS}-${GOARCH}${EXT}"
    GOOS="${GOOS}" GOARCH="${GOARCH}" go build -trimpath \
      -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}'" \
      -o "${OUT}" .
    echo "Built ${OUT}"

# Cross-compile helper (used by platform-specific recipes).
[private]
dist-go goos goarch:
    #!/usr/bin/env bash
    set -euo pipefail
    VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
    BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
    GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
    mkdir -p dist
    EXT=""
    if [[ "{{ goos }}" == "windows" ]]; then EXT=".exe"; fi
    OUT="dist/{{ bin }}-{{ goos }}-{{ goarch }}${EXT}"
    GOOS="{{ goos }}" GOARCH="{{ goarch }}" go build -trimpath \
      -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}'" \
      -o "${OUT}" .
    echo "Built ${OUT}"

build-linux-amd64: (dist-go "linux" "amd64")
build-linux-arm64: (dist-go "linux" "arm64")
build-darwin-amd64: (dist-go "darwin" "amd64")
build-darwin-arm64: (dist-go "darwin" "arm64")
build-windows-amd64: (dist-go "windows" "amd64")
build-windows-arm64: (dist-go "windows" "arm64")

build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 build-windows-arm64

alias build-amd64 := build-linux-amd64
alias build-arm64 := build-linux-arm64

release: build-all
    #!/usr/bin/env bash
    set -euo pipefail
    cd dist && sha256sum \
      {{ bin }}-linux-amd64 \
      {{ bin }}-linux-arm64 \
      {{ bin }}-darwin-amd64 \
      {{ bin }}-darwin-arm64 \
      {{ bin }}-windows-amd64.exe \
      {{ bin }}-windows-arm64.exe \
      > sha256sums.txt
    echo "Release artifacts in dist/"

run *args:
    go run . {{ args }}

test:
    go test -count=1 ./...

test-unit:
    go test -count=1 -short ./...

test-integration:
    go test -count=1 -tags integration ./...

vet:
    go vet ./...

fmt:
    gofmt -s -w .

fmt-check:
    test -z "$(gofmt -s -l .)"

tidy:
    go mod tidy

mod-verify:
    go mod verify

check: vet mod-verify test

ci: fmt-check check

clean:
    rm -f ./{{ bin }} ./{{ bin }}.exe
    rm -rf dist/
