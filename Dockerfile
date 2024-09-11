FROM golang:1.23.1-alpine3.20 AS builder

WORKDIR /app

COPY api/ ./
COPY .git/ ./
RUN apk update && apk add --no-cache git && \
    go install github.com/swaggo/swag/cmd/swag@latest && \
    swag init && \
    swag fmt && go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.versionTag=$(git rev-parse --short HEAD)" -trimpath -o api .


FROM alpine:latest

LABEL org.opencontainers.image.source=https://github.com/Clinical-Pharmacy-Saarland-University/abdataapi-go
LABEL org.opencontainers.image.description="ABDATA Database API"
LABEL org.opencontainers.image.licenses=MIT

RUN addgroup -S appgroup && adduser -S appuser -G appgroup && \
    mkdir -p /app /logs && \
    chown -R appuser:appgroup /app /logs \
    && chmod -R 755 /app \
    && chmod -R 755 /logs

COPY --from=builder /app/api /app/api
COPY api/cfg/prod_config.yml /app/config.yml

USER appuser
WORKDIR /app
ENTRYPOINT ["/app/api", "--config", "/app/config.yml"]

EXPOSE 3333