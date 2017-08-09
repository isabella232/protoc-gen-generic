#!/bin/bash
set -e
go build
protoc \
  -I/usr/local/include \
  -I. \
  -I$GOPATH/src \
  -I../identity/vendor \
  -I../identity/vendor/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
  --generic_out=. \
  identity.proto
