#!/bin/bash
# =============================================================================
# Build and push the current branch as a Docker image.
# =============================================================================
# Usage:
#   ./deploy/build-and-push-image.sh registry.example.com/sub2api:20260601
#
# Optional:
#   PLATFORM=linux/amd64 ./deploy/build-and-push-image.sh registry.example.com/sub2api:20260601
#   PUSH=false ./deploy/build-and-push-image.sh local/sub2api:test
#   NODE_IMAGE=m.daocloud.io/docker.io/library/node:24-alpine ./deploy/build-and-push-image.sh registry.example.com/sub2api:20260601
# =============================================================================

set -euo pipefail

IMAGE="${1:-${SUB2API_IMAGE:-}}"
PLATFORM="${PLATFORM:-}"
PUSH="${PUSH:-true}"

if [ -z "${IMAGE}" ]; then
    echo "Usage: $0 <image-ref>"
    echo "Example: $0 registry.example.com/sub2api:20260601"
    exit 1
fi

if command -v git >/dev/null 2>&1; then
    COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo docker)"
else
    COMMIT="docker"
fi

DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "Building image: ${IMAGE}"
echo "Commit: ${COMMIT}"
echo "Date: ${DATE}"

BUILD_ARGS=(
    --build-arg "COMMIT=${COMMIT}"
    --build-arg "DATE=${DATE}"
)

if [ -n "${NODE_IMAGE:-}" ]; then
    BUILD_ARGS+=(--build-arg "NODE_IMAGE=${NODE_IMAGE}")
fi
if [ -n "${GOLANG_IMAGE:-}" ]; then
    BUILD_ARGS+=(--build-arg "GOLANG_IMAGE=${GOLANG_IMAGE}")
fi
if [ -n "${ALPINE_IMAGE:-}" ]; then
    BUILD_ARGS+=(--build-arg "ALPINE_IMAGE=${ALPINE_IMAGE}")
fi
if [ -n "${POSTGRES_IMAGE:-}" ]; then
    BUILD_ARGS+=(--build-arg "POSTGRES_IMAGE=${POSTGRES_IMAGE}")
fi

if [ -n "${PLATFORM}" ]; then
    if [ "${PUSH}" = "true" ]; then
        docker buildx build \
            --platform "${PLATFORM}" \
            "${BUILD_ARGS[@]}" \
            -t "${IMAGE}" \
            --push \
            .
    else
        docker buildx build \
            --platform "${PLATFORM}" \
            "${BUILD_ARGS[@]}" \
            -t "${IMAGE}" \
            --load \
            .
    fi
else
    docker build \
        "${BUILD_ARGS[@]}" \
        -t "${IMAGE}" \
        .

    if [ "${PUSH}" = "true" ]; then
        docker push "${IMAGE}"
    fi
fi

echo "Done: ${IMAGE}"
