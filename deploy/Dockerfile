# syntax=docker/dockerfile:1
# Build from the repository root:
#   docker build -f deploy/Dockerfile --build-arg VERSION=$(git describe --tags --always) -t ggboard .

# ---- frontend ----
FROM node:24-alpine AS web
WORKDIR /src/web
# Chromium prerenders the public routes at build time (web/scripts/prerender.mjs); build stage only.
RUN apk add --no-cache chromium && npm install -g pnpm@12.4.1
ENV CHROME_PATH=/usr/bin/chromium
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml web/.pnpmfile.cjs ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# ---- backend ----
FROM golang:1.27-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/web/dist/ ./internal/static/dist/
ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/ggboard ./cmd/ggboard \
	&& mkdir -p /out/data

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ggboard /usr/local/bin/ggboard
COPY --from=build --chown=65532:65532 /out/data /data
ENV GGBOARD_DATA_DIR=/data \
	GGBOARD_SERVER_ADDR=:8080
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/ggboard"]
CMD ["serve"]
