# ── build stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS build
ARG BUILD_VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal/ internal/
COPY cmd/      cmd/
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.buildVersion=${BUILD_VERSION}" -o /out/server ./cmd/server

# ── runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=build /out/server ./server
COPY templates/ templates/
COPY static/    static/

EXPOSE 8080
CMD ["./server", "-addr=:8080"]
