# Production image: the single instantappoint binary (PocketBase + the
# embedded SPA) on distroless. Used by docker-compose.hetzner.yml.

# 1. The SPA. web/dist is embedded into the binary, so it has to exist before
#    `go build`. The VITE_META_* defaults in the source name the Meta app the
#    server's META_APP_ID points at; see README "Two build-time settings".
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. The binary. Go 1.27 is a hard requirement of PocketBase v0.40.6 (README).
#    Pure Go, no cgo.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/instantappoint ./cmd/server

# 3. Runtime. Distroless static, uid 65532 like neurocrow-hooks and
#    neurocrow-content: the deploy script chowns the pb_data volume to match.
#    time/tzdata is embedded in the binary, so no tzdata package is needed.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/instantappoint /app/instantappoint
USER 65532:65532
EXPOSE 8090
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=5 \
  CMD ["/app/instantappoint", "healthcheck"]
ENTRYPOINT ["/app/instantappoint"]
CMD ["serve", "--http=0.0.0.0:8090", "--dir=/pb_data"]
