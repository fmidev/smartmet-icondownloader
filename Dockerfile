FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o icon-grib-downloader -ldflags="-s -w -X 'main.version=$(git describe --tags --always || echo dev)'"

# Use a minimal alpine image for the final container
FROM alpine:3.18

# Install dependencies required for runtime
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/icon-grib-downloader /app/icon-grib-downloader

# Create a directory for downloaded data
RUN mkdir -p /data

# Set the volume for downloaded data
VOLUME /data

# Set the default output directory
ENV OUTPUT_DIR=/data

# Set the entrypoint to the binary
ENTRYPOINT ["/app/icon-grib-downloader"]

# Set default command to show help
CMD ["-help"]