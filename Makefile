.PHONY: help tidy deps proto fmt build run migrate sweep test lint docker up down

BINARY := dropcrate
IMAGE := dropcrate:latest

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

tidy: ## Resolve module dependencies
	go mod tidy

deps: ## Download and verify module dependencies
	go mod download && go mod verify

proto: ## Regenerate gRPC/protobuf stubs from proto/ (needs buf)
	buf lint && buf generate

fmt: ## Format all Go code
	go fmt ./...

build: ## Build the binary
	go build -o $(BINARY) .

run: ## Run the API server
	go run . serve

migrate: ## Apply database migrations
	go run . migrate

sweep: ## Reclaim expired files once and exit
	go run . sweep

test: ## Run tests
	go test ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

docker: ## Build the container image
	docker build -t $(IMAGE) .

up: ## Start local infrastructure (MySQL, Redis, MinIO)
	docker compose up -d

down: ## Stop local infrastructure
	docker compose down
