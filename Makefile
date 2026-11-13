GO ?= go
# VERSION overrides the version the program reports (by default persona-extension.json's).
VERSION ?=
LDFLAGS := $(if $(VERSION),-X main.version=$(VERSION))
PLATFORMS := linux-amd64 linux-arm64 darwin-arm64 darwin-amd64

.PHONY: all check fmt vet test build dist clean

all: check build

check: fmt vet test

fmt:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "not gofmt'ed:"; echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

test:
	$(GO) test -race -count=1 ./...

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/personal-agent-club ./cmd/personal-agent-club

# The binaries persona-extension.json names, one per platform.
dist:
	@for p in $(PLATFORMS); do \
		os=$${p%-*}; arch=$${p#*-}; \
		echo "bin/personal-agent-club-$$p"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/personal-agent-club-$$p ./cmd/personal-agent-club || exit 1; \
	done

clean:
	rm -rf bin dist
