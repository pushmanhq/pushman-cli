PDEV_API_URL ?= http://127.0.0.1:8080/v1
PDEV_BINARY ?= .bin/pushman-dev
PDEV_INSTALL_PATH ?= $(HOME)/.local/bin/pdev
PDEV_LDFLAGS = -X main.version=dev-local -X main.commit=working-tree -X main.date=local -X main.defaultBaseURL=$(PDEV_API_URL) -X main.credentialNamespace=dev -X main.automationTokenEnvironment=PUSHMAN_DEV_TOKEN

.PHONY: build-dev dev pdev install-dev test test-race

build-dev:
	mkdir -p $(dir $(PDEV_BINARY))
	go build -trimpath -ldflags "$(PDEV_LDFLAGS)" -o $(PDEV_BINARY) ./cmd/pushman

dev pdev: build-dev
	$(PDEV_BINARY) $(ARGS)

install-dev:
	mkdir -p $(dir $(PDEV_INSTALL_PATH))
	go build -trimpath -ldflags "$(PDEV_LDFLAGS)" -o $(PDEV_INSTALL_PATH) ./cmd/pushman

test:
	go test -p=1 -parallel=1 ./...
	go vet ./...

test-race:
	go test -race -p=1 -parallel=1 ./...
	go vet ./...
