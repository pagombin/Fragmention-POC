# Multi-stage build: the first stage builds the SPA, the second stage
# cross-compiles the Go binary with the SPA embedded, and the final stage
# ships a distroless image.

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --prefer-offline --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git make
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Overlay the freshly-built SPA into the embed dir so the Go binary picks
# it up at build time.
COPY --from=web /src/web/dist /src/internal/api/webui/dist
ARG VERSION=0.1.0
ARG COMMIT=unknown
ARG BUILDTIME=unknown
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -ldflags "-s -w \
      -X github.com/pagombin/fragmention-poc/internal/version.Version=${VERSION} \
      -X github.com/pagombin/fragmention-poc/internal/version.Commit=${COMMIT} \
      -X github.com/pagombin/fragmention-poc/internal/version.BuildTime=${BUILDTIME}" \
    -o /out/mfpoc ./cmd/mfpoc

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mfpoc /usr/local/bin/mfpoc
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/mfpoc"]
CMD ["server", "--config", "/etc/mfpoc/config.yaml"]
