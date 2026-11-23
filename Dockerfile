# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY cmd/ cmd/
COPY internal/ internal/
COPY configs/ configs/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/orbit ./cmd/orbit

# Final runtime image
FROM alpine:3.19

RUN apk --no-cache add ca-certificates curl tzdata && \
    addgroup -S orbit && adduser -S orbit -G orbit

WORKDIR /home/orbit

COPY --from=builder /app/orbit /usr/local/bin/orbit
COPY --from=builder /app/configs /home/orbit/configs

RUN mkdir -p /home/orbit/data && chown -R orbit:orbit /home/orbit

USER orbit

EXPOSE 2525 3525

HEALTHCHECK --interval=5s --timeout=3s --start-period=3s --retries=3 \
  CMD curl -f http://localhost:2525/live || exit 1

ENTRYPOINT ["/usr/local/bin/orbit"]
CMD ["-config", "configs/node-a.json", "-cluster", "configs/cluster.json"]
