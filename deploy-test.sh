#!/bin/bash

set -e

# Build des deux cibles dans un conteneur linux/amd64 (Rosetta/QEMU sur Mac) :
#  - beacon     -> NUC   (linux/amd64, gcc natif du conteneur)
#  - beacon.arm -> borne (linux/arm64, cross gcc-aarch64-linux-gnu)
# CGO obligatoire : core2 tire chai2010/webp. Un build natif macOS produirait
# un Mach-O inutilisable sur les cibles.
# Le monorepo entier (orbinea.example/) est monté : go.mod pointe vers ../../core2/*
# et ../nexus via replace.
SRC_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"   # .../Regia-core2/src/orbinea.example

echo "building binaries (docker)..."
docker run --rm --platform linux/amd64 \
  -v "$SRC_ROOT":/src \
  -v beacon-gomod:/go/pkg/mod \
  -v beacon-gocache:/root/.cache/go-build \
  -w /src/fluxia/beacon \
  golang:1.25-bookworm bash -c '
    set -e
    apt-get update -qq && apt-get install -y -qq gcc-aarch64-linux-gnu >/dev/null
    CGO_ENABLED=1 go build -o deployable/beacon backend/cmd/main.go
    GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc \
      go build -o deployable/beacon.arm backend/cmd/main.go
  '

echo "checking binaries..."
file deployable/beacon | grep -q "x86-64" || { echo "beacon: mauvaise arch"; exit 1; }
file deployable/beacon.arm | grep -q "aarch64" || { echo "beacon.arm: mauvaise arch"; exit 1; }

echo "copying files..."
scp deployable/beacon bacchus-09:/mnt/bacchus/volumes/fluxia-puppet/environments/testserver/modules/fluxia/files/beacon/beacon
scp deployable/beacon.arm bacchus-09:/mnt/bacchus/volumes/fluxia-puppet/environments/testserver/modules/fluxia/files/beacon/beacon.arm

#scp deployable/beacon.service bacchus-01:/mnt/bacchus/volumes/fluxia-puppet/environments/production/modules/fluxia/files/beacon/beacon.service

echo "cleaning binaries..."
rm -f deployable/beacon deployable/beacon.arm

echo "done!"
