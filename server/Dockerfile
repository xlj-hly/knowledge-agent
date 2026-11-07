# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26.10
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/api

FROM alpine:3.23
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN addgroup -S -g 10001 agent \
    && adduser -S -D -H -u 10001 -G agent agent \
    && mkdir -p /data/objects \
    && chown -R agent:agent /data
WORKDIR /app
COPY --from=build /out/server /app/agent-api
COPY --from=build /src/internal/infra/knowledge/WEKNORA_LICENSE /src/internal/infra/knowledge/THIRD_PARTY_NOTICES.md /app/licenses/
COPY --from=build /src/internal/infra/ai/providers/WEKNORA_LICENSE /src/internal/infra/ai/providers/ANYTHINGLLM_LICENSE /src/internal/infra/ai/providers/THIRD_PARTY_NOTICES.md /app/licenses/model-providers/
COPY --from=build /src/internal/infra/knowledge/assets/*LICENSE /src/internal/infra/knowledge/assets/*NOTICES.md /app/licenses/vector-providers/
COPY --from=build /src/internal/infra/knowledge/VECTOR_HTTP_NOTICES.md /app/licenses/vector-providers/
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -T 3 -O /dev/null http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/app/agent-api"]
