#!/usr/bin/env bash

GOOS=linux GOARCH=amd64 go build -o MCAuthProxy -ldflags "-s -w" && upx MCAuthProxy
GOOS=linux GOARCH=arm64 go build -o MCAuthProxy_arm64 -ldflags "-s -w" && upx MCAuthProxy_arm64
