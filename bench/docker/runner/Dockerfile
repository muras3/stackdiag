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
    ca-certificates \
    libfaketime

COPY --from=builder /usr/local/bin/stdiag /usr/local/bin/stdiag

# Generate certs and install bench CA into system trust store
COPY bench/docker/certs/generate.sh /tmp/generate.sh
RUN mkdir -p /etc/bench/certs && \
    cd /etc/bench/certs && \
    bash /tmp/generate.sh && \
    rm /tmp/generate.sh && \
    cp /etc/bench/certs/ca.crt /usr/local/share/ca-certificates/bench-ca.crt && \
    update-ca-certificates

WORKDIR /bench

# Keep container running for interactive use
CMD ["sleep", "infinity"]
