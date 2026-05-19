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

RUN make clean && make IMAGE_TAG=${IMAGE_TAG} goscape goscape-cli

## https://hub.docker.com/r/rockylinux/rockylinux
#FROM docker.io/rockylinux/rockylinux:10.1.20251123
#
#COPY --from=build /go/bin/goscape /usr/local/bin/goscape
#
#RUN groupadd --gid 65532 nonroot && \
#    useradd --uid 65532 --gid 65532 --comment nonroot nonroot
#
## Use numeric IDs for compatibility with runAsNonRoot in Kubernetes
#USER 65532:65532
#
#CMD ["goscape"]

# https://github.com/googlecontainertools/distroless
# nonroot, debug-nonroot
FROM gcr.io/distroless/static-debian13:debug-nonroot

# Use numeric IDs for compatibility with runAsNonRoot in Kubernetes
#USER 65532:65532

COPY --from=build /go/src/cmd/goscape/goscape /usr/bin/goscape
COPY --from=build /go/src/cmd/goscape-cli/goscape-cli /usr/bin/goscape-cli

# problem with entrypoint is we can't get a shell?
ENTRYPOINT ["/usr/bin/goscape"]
# Place default goscape args in CMD
#CMD ["--config.file=/etc/goscape/local-config.yaml"]

# https://friday-go.icu/golang/Go-Containerization-Best-Practices-Docker-Optimization

# TODO: opencontainer labels, EXPOSE
# https://github.com/opencontainers/image-spec/blob/main/annotations.md
# https://www.docker.com/blog/docker-best-practices-using-tags-and-labels-to-manage-docker-image-sprawl/
# https://docs.docker.com/engine/manage-resources/labels/#manage-labels-on-objects
# https://docs.docker.com/build/building/best-practices/#from
