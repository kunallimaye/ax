.PHONY: all build proto test clean install images run-on-run deploy-adk-cloudrun teardown-cloudrun

# Default container registry for docker
export KO_DOCKER_REPO ?= gcr.io/ax-container-images

# Build all binaries
all: proto build

# Build binaries
build:
	@echo "Building ax..."
	@mkdir -p bin
	@go build -o bin/ax ./cmd/ax
	@echo "Build complete!"


# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@export PATH=$$PATH:$$(go env GOPATH)/bin && \
		protoc --go_out=. --go_opt=paths=source_relative \
		       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
		       proto/ax.proto proto/content.proto
	@python3 -m grpc_tools.protoc -I. --python_out=python --grpc_python_out=python proto/ax.proto proto/content.proto
	@echo "Protobuf generation complete!"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -rf eventlog/
	@echo "Clean complete!"

# Install ax to GOPATH/bin
install:
	@echo "Installing ax..."
	@go install ./cmd/ax
	@echo "Install complete!"


# Run remote agent example
run-remote:
	@go run ./examples/remote_agent

# Run a single prompt end-to-end against the ADK agent on Cloud Run.
# Deploys the agent (if needed) and executes one turn via the cloudrun runtime.
# Usage: make run-on-run PROMPT="what is the weather in London, England?"
# All automation lives in scripts/; this target only delegates.
run-on-run:
	@scripts/run-on-run.sh "$(PROMPT)"

# Build + push + deploy the ADK weather agent to Cloud Run.
deploy-adk-cloudrun:
	@scripts/deploy-adk-cloudrun.sh

# Remove the Cloud Run service (and image with AX_DELETE_IMAGE=1).
teardown-cloudrun:
	@scripts/teardown-cloudrun.sh

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Dependencies installed!"

clean-logs:
	@echo "Cleaning the event logs..."
	rm -rf ./eventlog
	mkdir ./eventlog

ax-image:
	@echo "Building container image with ko..."
	ko build --base-import-paths ./cmd/ax

ax-server-image:
	@echo "Building ax-server container image with ko..."
	ko build --base-import-paths ./cmd/ax

ax-shell-image:
	# Used to debug ax servers within a cluster.
	@echo "Building ax shell container image with ko using busybox..."
	KO_DOCKER_REPO=$(KO_DOCKER_REPO)/ax-shell KO_DEFAULTBASEIMAGE=busybox:1.36 ko build --base-import-paths ./cmd/ax

# Build all container images
images: ax-image ax-shell-image
