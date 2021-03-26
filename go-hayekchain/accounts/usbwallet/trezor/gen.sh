#!/bin/env bash
protoc -I/usr/local/include:. --go_out=import_path=trezor:. messages.proto messages-common.proto messages-management.proto messages-hayekchain.proto
