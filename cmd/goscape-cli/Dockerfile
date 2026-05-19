ARG GO_VERSION=1.26

# https://hub.docker.com/_/golang
FROM docker.io/golang:${GO_VERSION} AS build
ARG IMAGE_TAG

WORKDIR /go/src

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading
# them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN make clean && make IMAGE_TAG=${IMAGE_TAG} goscape-cli


# https://github.com/googlecontainertools/distroless
# nonroot, debug-nonroot
FROM gcr.io/distroless/static-debian13:debug-nonroot

COPY --from=build /go/src/cmd/goscape-cli/goscape-cli /usr/bin/goscape-cli

ENTRYPOINT ["/usr/bin/goscape-cli"]
