# Stage 1: build binary
FROM golang:1.22 AS builder

WORKDIR /app

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically for Linux (GOOS=linux, arch lo define buildx)
RUN CGO_ENABLED=0 GOOS=linux go build -o app ./cmd/...

# Stage 2: minimal runtime image
FROM gcr.io/distroless/base-debian12

WORKDIR /app
COPY --from=builder /app/app /app/app

USER nonroot:nonroot

ENTRYPOINT ["/app/app"]
