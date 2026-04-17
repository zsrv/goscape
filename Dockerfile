# https://hub.docker.com/_/golang
FROM docker.io/golang:1.26.2-trixie AS build

WORKDIR /go/src/goscape

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading
# them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -o /go/bin/goscape ./cmd/goscape

# https://hub.docker.com/r/rockylinux/rockylinux
FROM docker.io/rockylinux/rockylinux:10.1.20251123

COPY --from=build /go/bin/goscape /usr/local/bin/goscape

RUN groupadd --gid 65532 nonroot && \
    useradd --uid 65532 --gid 65532 --comment nonroot nonroot

# Use numeric IDs for compatibility with runAsNonRoot in Kubernetes
USER 65532:65532

CMD ["goscape"]
