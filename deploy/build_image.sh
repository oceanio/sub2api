#!/usr/bin/env bash
# 本地构建镜像的快速脚本，避免在命令行反复输入构建参数。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# 选择 Dockerfile：
#   默认 Dockerfile           — 镜像自带前端 dist（开箱即用）
#   NOEMBED=1 或 DOCKERFILE=Dockerfile.noembed
#                             — 不构建前端，前端必须运行时从 /app/data/public/ 提供
DOCKERFILE="${DOCKERFILE:-Dockerfile}"
if [[ "${NOEMBED:-0}" == "1" ]]; then
    DOCKERFILE="Dockerfile.noembed"
fi

docker build -t sub2api:latest \
    --build-arg GOPROXY=https://goproxy.cn,direct \
    --build-arg GOSUMDB=sum.golang.google.cn \
    -f "${REPO_ROOT}/${DOCKERFILE}" \
    "${REPO_ROOT}"
