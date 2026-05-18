#!/bin/sh

export CGO_ENABLED=0

go build -trimpath -ldflags '-s -w' -o goscape     ./cmd/goscape
go build -trimpath -ldflags '-s -w' -o goscape-cli ./cmd/goscape-cli
