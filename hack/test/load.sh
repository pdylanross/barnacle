#!/usr/bin/env bash
#
# Load test script for barnacle
# Pulls a large number of Docker images through the barnacle proxy
#

set -euo pipefail

REGISTRY="${BARNACLE_REGISTRY:-localhost:8080/dockerio}"
NOPRUNE="${NOPRUNE:-no}"

echo "=== Barnacle Load Test ==="
echo "Registry: ${REGISTRY}"
echo ""

# Clean up Docker to ensure fresh pulls
echo "=== Cleaning up Docker ==="
if [ "$NOPRUNE" != "yes" ]; then
  docker system prune -a -f
fi
echo ""

# Function to pull an image and report status
pull_image() {
    local image="$1"
    echo "Pulling: ${REGISTRY}/${image}"
    if docker pull "${REGISTRY}/${image}"; then
        echo "  SUCCESS: ${image}"
    else
        echo "  FAILED: ${image}"
    fi
}

echo "=== Pulling Python Images ==="
# Python versions: 3.13, 3.12, 3.11, 3.10, 3.9
PYTHON_VERSIONS=("3.13" "3.12" "3.11" "3.10" "3.9")
PYTHON_VARIANTS=("" "-slim" "-slim-bookworm" "-slim-bullseye" "-bookworm" "-bullseye" "-alpine")

for version in "${PYTHON_VERSIONS[@]}"; do
    for variant in "${PYTHON_VARIANTS[@]}"; do
        pull_image "python:${version}${variant}"
    done
done

echo ""
echo "=== Pulling Golang Images ==="
# Go versions: 1.23, 1.22, 1.21, 1.20, 1.19
GO_VERSIONS=("1.23" "1.22" "1.21" "1.20" "1.19")
GO_VARIANTS=("" "-bookworm" "-bullseye" "-alpine")

for version in "${GO_VERSIONS[@]}"; do
    for variant in "${GO_VARIANTS[@]}"; do
        pull_image "golang:${version}${variant}"
    done
done

echo ""
echo "=== Pulling Node.js Images ==="
# Node versions: 22, 21, 20, 18, 16
NODE_VERSIONS=("22" "21" "20" "18" "16")
NODE_VARIANTS=("" "-slim" "-slim-bookworm" "-slim-bullseye" "-bookworm" "-bullseye" "-alpine")

for version in "${NODE_VERSIONS[@]}"; do
    for variant in "${NODE_VARIANTS[@]}"; do
        pull_image "node:${version}${variant}"
    done
done

echo ""
echo "=== Pulling Debian Base Images ==="
DEBIAN_VERSIONS=("bookworm" "bookworm-slim" "bullseye" "bullseye-slim" "buster" "buster-slim")

for version in "${DEBIAN_VERSIONS[@]}"; do
    pull_image "debian:${version}"
done

echo ""
echo "=== Pulling Alpine Base Images ==="
ALPINE_VERSIONS=("3.20" "3.19" "3.18" "3.17" "edge")

for version in "${ALPINE_VERSIONS[@]}"; do
    pull_image "alpine:${version}"
done

echo ""
echo "=== Pulling Ubuntu Base Images ==="
UBUNTU_VERSIONS=("24.04" "22.04" "20.04" "noble" "jammy" "focal")

for version in "${UBUNTU_VERSIONS[@]}"; do
    pull_image "ubuntu:${version}"
done

echo ""
echo "=== Pulling Redis Images ==="
REDIS_VERSIONS=("7" "7-alpine" "6" "6-alpine")

for version in "${REDIS_VERSIONS[@]}"; do
    pull_image "redis:${version}"
done

echo ""
echo "=== Pulling PostgreSQL Images ==="
POSTGRES_VERSIONS=("16" "16-alpine" "15" "15-alpine" "14" "14-alpine")

for version in "${POSTGRES_VERSIONS[@]}"; do
    pull_image "postgres:${version}"
done

echo ""
echo "=== Pulling Nginx Images ==="
NGINX_VERSIONS=("latest" "alpine" "1.27" "1.27-alpine" "1.26" "1.26-alpine")

for version in "${NGINX_VERSIONS[@]}"; do
    pull_image "nginx:${version}"
done

echo ""
echo "=== Pulling Pytorch Images ==="
PYTORCH_VERSIONS=("latest" "2.10.0-cuda13.0-cudnn9-runtime" "2.9.0-cuda13.0-cudnn9-runtime" "2.9.0-cuda13.0-cudnn9-runtime" "2.8.0-cuda12.9-cudnn9-runtime" "2.8.0-cuda12.8-cudnn9-runtime" "2.8.0-cuda12.6-cudnn9-runtime")

for version in "${PYTORCH_VERSIONS[@]}"; do
    pull_image "pytorch/pytorch:${version}"
done

echo ""
echo "=== Pulling opensearch Images ==="
OPENSEARCH_VERSIONS=("latest" "3", "2")

for version in "${OPENSEARCH_VERSIONS[@]}"; do
    pull_image "opensearchproject/opensearch:${version}"
done

echo ""
echo "=== Load Test Complete ==="
echo "Total images pulled from: ${REGISTRY}"
