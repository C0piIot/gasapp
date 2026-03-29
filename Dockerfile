# ── build stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY internal/ internal/
COPY cmd/      cmd/
RUN go mod tidy
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/server  ./cmd/server
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/update  ./cmd/update

# ── runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=build /out/server  ./server
COPY --from=build /out/update  ./update

# Static assets and templates are mounted at runtime (see docker-compose.yaml).
# The update binary is invoked by cron / a sidecar; only the server runs here.
EXPOSE 8080
CMD ["./server", "-addr=:8080"]
