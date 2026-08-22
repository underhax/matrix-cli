VERSION ?= dev

.PHONY: check test coverage vulncheck docker trivy verify build

check:
	@test -z "$$(gofmt -s -l .)" || (echo "Unformatted files found. Run 'gofmt -s -w .' to fix them." && false)
	golangci-lint run ./...
	go build -tags goolm ./...

test:
	go test -tags goolm -v -race ./...

coverage:
	go test -tags goolm -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

vulncheck:
	govulncheck -tags goolm ./...

docker:
	hadolint docker/Dockerfile

trivy:
	trivy fs --severity CRITICAL,HIGH .

verify: check test vulncheck trivy docker

build:
	CGO_ENABLED=1 go build -trimpath -tags goolm -ldflags "-s -w -X main.AppVersion=$(VERSION)" -o matrix-cli ./cmd/matrix-cli
