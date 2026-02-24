#!/bin/bash

set -ex

if [ -z "${GOPATH:-}" ]; then
    eval "$(go env | grep GOPATH)"
fi

export OUTPUT=bin/golangci-lint

if [ ! -f "$OUTPUT" ]
then
    curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(go env GOPATH)/bin v1.64.8
fi