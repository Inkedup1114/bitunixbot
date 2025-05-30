FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -buildvcs=false -trimpath -ldflags="-s -w" -o /bitrader ./cmd/bitrader

FROM alpine:3.20
RUN apk add --no-cache ca-certificates libonnxruntime && \
    adduser -D -u 65532 -g 65532 appuser
COPY --from=build /bitrader /usr/local/bin/
RUN chmod +x /usr/local/bin/bitrader
USER 65532:65532
WORKDIR /srv/data
CMD ["/usr/local/bin/bitrader"]
