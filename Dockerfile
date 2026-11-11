FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api

FROM alpine:3

WORKDIR /app

RUN adduser -D -u 10001 app

COPY --from=builder --chown=app:app /app/bin/api /app/api

USER app

EXPOSE 8080

CMD ["/app/api"]
