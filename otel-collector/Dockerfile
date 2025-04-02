FROM golang:1.21 AS builder

WORKDIR /build

# Copy module files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy extension code
COPY extension/ extension/

# Build the OpenTelemetry Collector with our custom components
COPY builder/builder.go .
COPY builder/go.mod builder/go.sum ./builder/
RUN cd builder && go run builder.go

# Final image
FROM alpine:3.18

RUN apk --no-cache add ca-certificates libc6-compat && \
    update-ca-certificates

# Copy the built collector binary
COPY --from=builder /build/collector /bin/

# Create non-root user
RUN addgroup -g 10001 otel && \
    adduser -D -u 10001 -G otel otel

USER otel

EXPOSE 4317 4318 55678 55679
VOLUME ["/etc/otel-collector"]
ENTRYPOINT ["/bin/collector"]
CMD ["--config=/etc/otel-collector/otel-config.yaml"]