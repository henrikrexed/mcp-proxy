FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /mcp-otel-proxy ./cmd/mcp-otel-proxy

FROM alpine:3.21

RUN apk --no-cache add ca-certificates && \
    adduser -D -u 1000 appuser

COPY --from=builder /mcp-otel-proxy /usr/local/bin/mcp-otel-proxy

USER appuser
EXPOSE 8080

ENTRYPOINT ["mcp-otel-proxy"]
