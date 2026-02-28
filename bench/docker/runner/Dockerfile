FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod ./
RUN go mod download 2>/dev/null || true
COPY . .

RUN CGO_ENABLED=0 go build -o /usr/local/bin/stdiag ./cmd/stackdiag

FROM alpine:3.20

RUN apk add --no-cache \
    bind-tools \
    openssl \
    curl \
    bash \
    ca-certificates

COPY --from=builder /usr/local/bin/stdiag /usr/local/bin/stdiag

# CA cert is mounted at runtime; install it at container start
COPY bench/docker/runner/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

WORKDIR /bench

ENTRYPOINT ["/entrypoint.sh"]
CMD ["sleep", "infinity"]
