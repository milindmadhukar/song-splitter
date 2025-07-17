# Stage 1: Build the Go binary
FROM golang:1.23.6-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
# Download dependencies
RUN go mod download
RUN go mod verify

# Copy sources
COPY main.go .

# Build the Go app statically, for a linux amd64 target
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o /song-splitter .

# Stage 2: Final image
FROM alpine:latest

# Install ffmpeg for video/audio processing
RUN apk add --no-cache ffmpeg

# Copy the built binary from the builder stage
COPY --from=builder /song-splitter /usr/local/bin/song-splitter

# Set working directory. Input/output files will be relative to this path.
WORKDIR /data

# The command will be provided via docker-compose or `docker run`
ENTRYPOINT ["song-splitter"]
