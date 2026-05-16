#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。
# 默认走 buildx + 本地持久缓存，重复构建命中 pnpm store / Go module / Go build cache。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

IMAGE_TAG="${IMAGE_TAG:-oceanio/matrixrouter:latest}"
BUILDER_NAME="${BUILDX_BUILDER:-matrixrouter-builder}"
CACHE_DIR="${BUILDX_CACHE_DIR:-/tmp/matrixrouter-buildx-cache}"
# 默认只构建 amd64（生产部署目标架构）。本地若需 arm64 走 QEMU 模拟会显著变慢，
# 通过 PLATFORM=linux/arm64 或 PLATFORM=linux/amd64,linux/arm64 显式覆盖。
PLATFORM="${PLATFORM:-linux/amd64}"

# 复用 docker-container builder（首次自动创建），以启用 cache mount 与 local cache。
if ! docker buildx inspect "${BUILDER_NAME}" >/dev/null 2>&1; then
    docker buildx create --name "${BUILDER_NAME}" --driver docker-container >/dev/null
fi

mkdir -p "${CACHE_DIR}"

docker buildx build -t "${IMAGE_TAG}" \
    --builder "${BUILDER_NAME}" \
    --platform "${PLATFORM}" \
    --build-arg GOPROXY=https://goproxy.cn,direct \
    --build-arg GOSUMDB=sum.golang.google.cn \
    --cache-from "type=local,src=${CACHE_DIR}" \
    --cache-to "type=local,dest=${CACHE_DIR},mode=max" \
    --load \
    -f "${REPO_ROOT}/Dockerfile" \
    "${REPO_ROOT}"
