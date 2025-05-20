# ----------- stage 1: build the static binary -----------
# Use the latest Go release specified in go.mod/toolchain so that `go build`
# inside the container matches local development.
FROM golang:1.23-alpine AS builder

# Enable CGO=0 so the resulting binary is fully static and therefore can run
# in the scratch/distroless image that follows.
ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src

# Pre-cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source and build
COPY . .

# `-s -w` strips the symbol table and debug information to minimise size.
RUN go build -ldflags "-s -w" -o azure-openai-proxy ./


# ----------- stage 2: thin runtime image -----------
# Distroless provides only the files strictly necessary to run a static binary
# (including CA certificates for outbound TLS) which keeps the final image
# very small while still supporting HTTPS calls required by the proxy.
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the binary from the builder stage
COPY --from=builder /src/azure-openai-proxy /proxy

# The proxy listens on 8081 by default (configurable via the PORT env var)
EXPOSE 8081

# Drop root privileges – the distroless image already defines the nonroot user.
USER nonroot:nonroot

# Launch!
ENTRYPOINT ["/proxy"]
