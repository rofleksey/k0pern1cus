.PHONY: all
all: gen build run

.PHONY: clean
clean:
	@go clean

.PHONY: gen
gen:
	@echo "Generating dependency files..."
	@go generate ./...

.PHONY: lint
lint:
	@npx @redocly/cli lint
	@npx golangci-lint run

.PHONY: build
build:
	@echo "Building application..."
	@go build -ldflags "-X k0pern1cus/pkg/build.Tag=${GIT_TAG}" .

.PHONY: run
run:
	@./k0pern1cus
