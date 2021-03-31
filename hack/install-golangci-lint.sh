#!/bin/bash

set -ex

if [ -z "${GOPATH:-}" ]; then
    eval "$(go env | grep GOPATH)"
fi

OUTPUT=bin/golangci-lint
VERSION=v1.39.0

if [ ! -f "$OUTPUT" ]
then
    curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(go env GOPATH)/bin $(VERSION)
fi