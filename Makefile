# -------------------------
# Simple helper targets to build and run the proxy
# -------------------------

APP            ?= azure-openai-proxy
IMAGE          ?= $(APP):latest
PORT           ?= 8081

.PHONY: all build image run docker-run clean

# Build the local binary (static)
build:
	CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o $(APP) ./

# Build the container image using the multi-stage Dockerfile in the repo
image:
	docker build -t $(IMAGE) .

# Shortcut: build & run the image exposing the PORT environment variable
docker-run: image
	docker run -dit --rm -e PORT=$(PORT) -e TARGET_URL=$(TARGET_URL) -p "127.0.0.1:$(PORT)":$(PORT) $(IMAGE)

# Run locally (without Docker) – requires TARGET_URL to be provided
run:
	go run .

clean:
	rm -f $(APP)
