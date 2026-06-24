FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /dockify ./cmd/dockify

FROM alpine:3.21

RUN apk add --no-cache ca-certificates curl openssh-client

RUN addgroup -S dockify && adduser -S dockify -G dockify

COPY --from=builder /dockify /usr/local/bin/dockify

ENV DOCKIFY_HOST=0.0.0.0
ENV DOCKIFY_PORT=8080
ENV DOCKIFY_DATA_DIR=/var/lib/dockify

RUN mkdir -p /var/lib/dockify/keys && chown -R dockify:dockify /var/lib/dockify

USER dockify
EXPOSE 8080

ENTRYPOINT ["dockify", "serve"]
