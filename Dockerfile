FROM alpine:3.21

RUN apk --no-cache add ca-certificates &&     adduser -D -u 1000 appuser

COPY mcp-otel-proxy /usr/local/bin/mcp-otel-proxy

USER appuser
EXPOSE 8080

ENTRYPOINT ["mcp-otel-proxy"]
