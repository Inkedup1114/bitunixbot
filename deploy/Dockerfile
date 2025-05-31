FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -buildvcs=false -trimpath -ldflags="-s -w" -o /bitrader ./cmd/bitrader

FROM alpine:3.20
# Pin specific versions for reproducible builds
RUN apk add --no-cache \
    ca-certificates=20240705-r0 \
    python3=3.12.8-r1 \
    py3-pip=24.0-r2 \
    py3-numpy=1.24.4-r0 && \
    adduser -D -u 65532 -g 65532 appuser

# Install ONNX runtime for Python with specific version
RUN pip3 install --no-cache-dir onnxruntime==1.17.3

COPY --from=build /bitrader /usr/local/bin/
RUN chmod +x /usr/local/bin/bitrader

# Pre-create inference script directory and set permissions
RUN mkdir -p /srv/data && chown 65532:65532 /srv/data

USER 65532:65532
WORKDIR /srv/data
CMD ["/usr/local/bin/bitrader"]
