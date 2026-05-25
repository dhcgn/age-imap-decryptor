FROM golang:1.26.1-alpine AS builder

ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.buildVersion=${VERSION}" \
    -o /age-imap-decryptor ./cmd/age-imap-decryptor

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /age-imap-decryptor /age-imap-decryptor

ENTRYPOINT ["/age-imap-decryptor"]
