ARG BUILD_FROM

# ---- Go build stage ----
FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY app/ ./
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/uiwled ./cmd/uiwled

# ---- Runtime stage ----
FROM $BUILD_FROM

RUN apk add --no-cache openssh-client ca-certificates tzdata

COPY --from=builder /out/uiwled /usr/local/bin/uiwled
COPY run.sh /run.sh
RUN chmod +x /run.sh /usr/local/bin/uiwled

CMD ["/run.sh"]
