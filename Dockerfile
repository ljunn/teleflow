FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS backend
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist/ ./internal/web/dist/
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/ljunn/teleflow/internal/version.Version=${VERSION} -X github.com/ljunn/teleflow/internal/version.Commit=${COMMIT} -X github.com/ljunn/teleflow/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/teleflow ./cmd/teleflow

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S teleflow && adduser -S -G teleflow -h /var/lib/teleflow teleflow
COPY --from=backend /out/teleflow /usr/local/bin/teleflow
RUN mkdir -p /var/lib/teleflow && chown -R teleflow:teleflow /var/lib/teleflow
USER teleflow
WORKDIR /var/lib/teleflow
ENV TELEFLOW_ADDR=:8080 TELEFLOW_DATA_DIR=/var/lib/teleflow
EXPOSE 8080
VOLUME ["/var/lib/teleflow"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health/ready >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/teleflow"]
