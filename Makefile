# Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
BINARY=go-uptime
DOCKER_IMAGE ?= jniltinho/go-uptime
DOCKER_PLATFORMS ?= linux/amd64,linux/arm64
VERSION ?= dev
DIST := dist
RELEASE_ARCHS := amd64 arm64
# Versão, commit e data gravados no binário, mostrados por `go-uptime version`
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
# Cada -X entre aspas simples, senão uma versão com espaço partiria o -ldflags; e só o alfabeto de uma versão é aceito,
# porque o valor é interpolado num comando do shell
LDFLAGS := -s -w -X 'github.com/jniltinho/go-uptime/v7/cmd.Version=$(VERSION)' -X 'github.com/jniltinho/go-uptime/v7/cmd.GitCommit=$(GIT_COMMIT)' -X 'github.com/jniltinho/go-uptime/v7/cmd.BuildDate=$(BUILD_DATE)'

.PHONY: check-version
check-version:
	@printf '%s' '$(subst ','\'',$(VERSION))' | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._+-]*$$' || { echo "VERSION inválida: use só letras, dígitos, ponto, hífen, sublinhado e +"; exit 1; }

.PHONY: install
install:
	go build -v -o $(BINARY) .

.PHONY: run
run:
	ENVIRONMENT=dev GATUS_CONFIG_PATH=./config.yaml go run .

.PHONY: run-binary
run-binary:
	ENVIRONMENT=dev GATUS_CONFIG_PATH=./config.yaml ./$(BINARY)

.PHONY: clean
clean:
	rm $(BINARY)

.PHONY: test
test:
	go test ./... -cover

.PHONY: build
build: check-version
	@mkdir -p $(DIST)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY) .

# Todo o código Go do projeto; third_party são módulos de terceiros, com o estilo deles
GO_FILES = $(shell git ls-files --cached --others --exclude-standard -- '*.go' | grep -v '^third_party/')

.PHONY: fmt
fmt:
	@gofmt -w $(GO_FILES)

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint: vet
	@unformatted=$$(gofmt -l $(GO_FILES)); \
	if [ -n "$$unformatted" ]; then echo "Arquivos precisando de gofmt (execute: make fmt):"; echo "$$unformatted"; exit 1; fi

.PHONY: release-cross
release-cross: check-version
	@rm -rf $(DIST)/pkg
	@for arch in $(RELEASE_ARCHS); do \
		mkdir -p $(DIST)/pkg/linux_$$arch && \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/pkg/linux_$$arch/$(BINARY) . && \
		tar -czf $(DIST)/$(BINARY)_$(VERSION)_linux_$$arch.tar.gz -C $(DIST)/pkg/linux_$$arch $(BINARY) -C $(CURDIR) config.yaml LICENSE NOTICE README.md && \
		echo "  $(DIST)/$(BINARY)_$(VERSION)_linux_$$arch.tar.gz" || exit 1; \
	done


# Publica a imagem multi-arquitetura no Docker Hub a partir desta máquina (exige docker login)
.PHONY: docker-release
docker-release:
	@[ "$(VERSION)" != "dev" ] || { echo "Defina a versão: make docker-release VERSION=6.0.0"; exit 1; }
	$(MAKE) release-cross VERSION=$(VERSION)
	docker buildx build -f Dockerfile.release --platform $(DOCKER_PLATFORMS) -t $(DOCKER_IMAGE):v$(VERSION) --push .

##########
# Docker #
##########

.PHONY: docker-build
docker-build: check-version
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_IMAGE):$(VERSION) .

.PHONY: docker-run
docker-run:
	docker run -p 8080:8080 --name go-uptime $(DOCKER_IMAGE):$(VERSION)

.PHONY: docker-build-and-run
docker-build-and-run: docker-build docker-run


#############
# Front end #
#############

.PHONY: frontend-install
frontend-install:
	npm --prefix web/app install

.PHONY: frontend-build
frontend-build:
	npm --prefix web/app run build

.PHONY: frontend-dev
frontend-dev:
	npm --prefix web/app run serve
