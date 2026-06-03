#!/bin/bash
# =============================================================================
# Package deployment files for servers that should not receive source code.
# =============================================================================
# Usage:
#   ./deploy/package-deploy-bundle.sh
#   BUNDLE_NAME=sub2api-deploy-20260601.tar.gz ./deploy/package-deploy-bundle.sh
# =============================================================================

set -euo pipefail

BUNDLE_NAME="${BUNDLE_NAME:-sub2api-deploy-bundle.tar.gz}"
TMP_BASE="${TMPDIR:-/tmp}"
if [ ! -d "${TMP_BASE}" ] || [ ! -w "${TMP_BASE}" ]; then
    TMP_BASE="/tmp"
fi
TMP_DIR="$(mktemp -d "${TMP_BASE%/}/sub2api-deploy-bundle.XXXXXX")"
BUNDLE_DIR="${TMP_DIR}/sub2api-deploy"

cleanup() {
    rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

mkdir -p "${BUNDLE_DIR}"

cp deploy/docker-compose.local.yml "${BUNDLE_DIR}/docker-compose.local.yml"
cp deploy/.env.example "${BUNDLE_DIR}/.env.example"
cp deploy/docker-deploy.sh "${BUNDLE_DIR}/docker-deploy.sh"
cp deploy/BRANCH_DOCKER_DEPLOY_CN.md "${BUNDLE_DIR}/README_CN.md"

chmod +x "${BUNDLE_DIR}/docker-deploy.sh"

tar -C "${TMP_DIR}" -czf "${BUNDLE_NAME}" sub2api-deploy

echo "Created ${BUNDLE_NAME}"
echo "Target server:"
echo "  tar xzf ${BUNDLE_NAME}"
echo "  cd sub2api-deploy"
echo "  SUB2API_IMAGE=registry.example.com/sub2api:tag SUB2API_INSTANCE=sub2api-test SUB2API_PORT=18080 ./docker-deploy.sh"
echo "  docker compose up -d"
