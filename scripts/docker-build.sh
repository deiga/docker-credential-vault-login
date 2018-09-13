#!/usr/bin/env bash

set -e

TOOL="docker-credential-vault-login"
REPO="gitlab.morningconsult.com/mci/${TOOL}"
BIN_DIR="bin/local"
ROOT=$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )

cd "${ROOT}"

mkdir -p "${ROOT}/bin/local"

echo "==> Building Docker image..."
IMAGE=$(docker build -q .)

echo "==> Building the binary..."
CONTAINER_ID=$(docker run --rm --detach --tty --env TARGET_GOOS=${TARGET_GOOS} --env TARGET_GOARCH=${TARGET_GOARCH} ${IMAGE})

docker cp "${CONTAINER_ID}:/go/src/${REPO}/${BIN_DIR}/${TOOL}" "${ROOT}/${BIN_DIR}"

docker kill "${CONTAINER_ID}" > /dev/null

echo "==> Done. The binary can be found in:  ${ROOT}/${BIN_DIR}/${TOOL}"