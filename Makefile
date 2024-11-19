GOCMD=go

.PHONY: all
all: help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: tidy
tidy: ## Run go mod tidy to organize dependencies.
	@$(GOCMD) mod tidy

.PHONY: test
test: ## run unit test applications.
	@$(GOCMD) test -race ./...

.PHONY: run
run: ## Run run application locally.
	@$(GOCMD) run -race cmd/serverd/main.go

.PHONY: resize-images-sync
resize-images-sync: ## post image urls using resize endpoint
	@curl -vv \
	http://localhost:8080/v1/resize?async=false \
	--request POST \
	-H "Content-Type: application/json" \
	-d @req.json

.PHONY: resize-images-async
resize-images-async: ## post image urls using asynchronous resize endpoint
	@curl -vv \
	http://localhost:8080/v1/resize?async=true \
	--request POST \
	-H "Content-Type: application/json" \
	-d @req.json

.PHONY: resize-images-async-two
resize-images-async-two: ## post image urls using resize endpoint but repeating 1 images
	@curl -vv \
	http://localhost:8080/v1/resize?async=true \
	--request POST \
	-H "Content-Type: application/json" \
	-d @req2.json
