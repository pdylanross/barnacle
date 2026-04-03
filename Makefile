.PHONY: all tools fmt swagger test e2e lint clean run local-up local-down open-local-redisinsight local-cluster-build local-cluster-up local-cluster-down local-cluster-clean local-cluster-healthcheck local-cluster-logs open-local-cluster-redisinsight e2e-setup e2e-deploy e2e-generate e2e-test e2e-clean e2e-imagegen-build

# Default target: format, generate swagger docs, and build
all: fmt swagger build

# Install required development tools
# See .claude/skills/environment-dependencies/SKILL.md for details
tools:
	@echo "Installing development tools..."
	go install github.com/swaggo/swag/cmd/swag@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@echo "All tools installed successfully"

# Generate swagger documentation
swagger:
	@echo "Generating swagger docs..."
	@swag init -g main.go --parseInternal -d ./cmd/barnacle/,./internal/routes/,./pkg/api/,./internal/tk/httptk/ -o docs

# Build the project
build:
	@echo "Building..."
	@go build -v ./...

# Run the barnacle CLI with optional arguments
# Usage: make run serve
#        make run -- serve --configDir /tmp/test  (use -- before flags)
run:
	@go run ./cmd/barnacle $(filter-out run,$(MAKECMDGOALS))

# Format code using goimports
fmt:
	@echo "Formatting code..."
	@goimports -w .
	@gofmt -s -w .

# Run unit tests (excluding e2e)
test:
	@echo "Running unit tests..."
	@go test -v $$(go list ./... | grep -v /test/e2e) -race

# Run end-to-end tests (simple mode - requires manual setup)
e2e:
	@echo "Running e2e tests..."
	@go test -v -tags=e2e ./test/e2e/...

# E2E Variables
E2E_WORKERS ?= 10
E2E_ITERATIONS ?= 10000
E2E_VARIANTS ?= 100
E2E_MINIKUBE_IP := $(shell minikube ip -p barnacle-e2e 2>/dev/null || echo "localhost")
E2E_REGISTRY ?= $(E2E_MINIKUBE_IP):30500
E2E_MANIFEST ?= e2e-images.json
E2E_KUBE_QPS ?= 20000
E2E_KUBE_BURST ?= 40000
E2E_RESULTS ?= ./test/e2e/out

# Create minikube cluster, build barnacle image, and deploy infrastructure
e2e-setup:
	@echo "Setting up e2e environment..."
	@./hack/e2e/minikube/setup.sh setup

# Deploy barnacle + redis + local registry (assumes cluster exists)
e2e-deploy:
	@echo "Deploying e2e infrastructure..."
	@./hack/e2e/minikube/setup.sh deploy

# Build the e2e image generator tool
e2e-imagegen-build:
	@echo "Building e2e image generator..."
	@go build -o bin/e2e-imagegen ./cmd/e2e-imagegen

# Generate test images (standalone, can re-run)
# Usage: make e2e-generate
#        make e2e-generate E2E_VARIANTS=50 E2E_REGISTRY=192.168.49.2:30500
e2e-generate: e2e-imagegen-build
	@echo "Generating $(E2E_VARIANTS) test images to $(E2E_REGISTRY)..."
	@./bin/e2e-imagegen \
		-registry $(E2E_REGISTRY) \
		-variants $(E2E_VARIANTS) \
		-output $(E2E_MANIFEST)

# Run e2e tests against existing images
# Usage: make e2e-test
#        make e2e-test E2E_WORKERS=20 E2E_ITERATIONS=50000
e2e-test:
	@mkdir -p $(E2E_RESULTS)
	@echo "Running e2e tests with $(E2E_WORKERS) workers, $(E2E_ITERATIONS) iterations..."
	@go test -v -tags=e2e ./test/e2e/... \
		-workers=$(E2E_WORKERS) \
		-iterations=$(E2E_ITERATIONS) \
		-manifest=$(CURDIR)/$(E2E_MANIFEST) \
		-barnacle-node-addr=$(E2E_MINIKUBE_IP):30080 \
		-kube-qps=$(E2E_KUBE_QPS) \
		-kube-burst=$(E2E_KUBE_BURST) \
		-results $(CURDIR)/$(E2E_RESULTS) \
		-timeout 0

# Teardown the e2e cluster
e2e-clean:
	@echo "Cleaning up e2e environment..."
	@./hack/e2e/minikube/setup.sh teardown

# Run linter
lint:
	@echo "Running golangci-lint..."
	@golangci-lint run --fix

# Start local development dependencies
local-up:
	@echo "Starting local dependencies..."
	@docker compose -f hack/local/docker-compose.yml up -d --remove-orphans


# Open RedisInsight dashboard for local environment
open-local-redisinsight:
	@xdg-open http://localhost:5540

# Stop local development dependencies
local-down:
	@echo "Stopping local dependencies..."
	@docker compose -f hack/local/docker-compose.yml down

# Build the local clustered environment
local-cluster-build:
	@echo "Building local cluster..."
	@docker compose -f hack/local-clustered/docker-compose.yml build

# Start the local clustered environment
local-cluster-up:
	@echo "Starting local cluster..."
	@docker compose -f hack/local-clustered/docker-compose.yml up -d --remove-orphans

# Stop the local clustered environment
local-cluster-down:
	@echo "Stopping local cluster..."
	@docker compose -f hack/local-clustered/docker-compose.yml down

# Tear down local cluster, removing networks and volumes
local-cluster-clean:
	@echo "Removing local cluster volumes and networks..."
	@docker compose -f hack/local-clustered/docker-compose.yml down -v --remove-orphans

# Open RedisInsight dashboard for local cluster environment
open-local-cluster-redisinsight:
	@xdg-open http://localhost:5541

# Follow logs for all local cluster barnacle nodes
local-cluster-logs:
	@docker compose -f hack/local-clustered/docker-compose.yml logs -f barnacle-1 barnacle-2 barnacle-3

# Check health of all local cluster nodes
local-cluster-healthcheck:
	@echo "barnacle-1:"; @curl -s http://localhost:8081/healthz; @echo ""
	@echo "barnacle-2:"; @curl -s http://localhost:8082/healthz; @echo ""
	@echo "barnacle-3:"; @curl -s http://localhost:8083/healthz; @echo ""

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@go clean
	@rm -f coverage.out

# Catch-all target to prevent make from complaining about unknown targets
# This allows arguments to be passed after 'make run'
%:
	@: