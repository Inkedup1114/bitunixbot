FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /bitrader ./cmd/bitrader

FROM alpine:3.20
RUN apk add --no-cache libonnxruntime
COPY --from=build /bitrader /usr/local/bin/
WORKDIR /work
CMD ["/usr/local/bin/bitrader"]
