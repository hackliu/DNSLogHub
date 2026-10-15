#!/usr/bin/env bash
# DNSLogHUB 一键交叉编译脚本
# 用法: ./build.sh [clean]
# 产出: dist/ 下各平台二进制 + tar.gz 压缩包 + SHA256 校验和（含邀请码生成工具）
set -euo pipefail

if [[ "${1:-}" == "clean" ]]; then
  rm -rf dist
  echo "已清理 dist/"
  exit 0
fi

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}"

DIST="dist"
rm -rf "$DIST"
mkdir -p "$DIST"

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "unavailable"
  fi
}

targets=(
  linux/amd64
  linux/arm64
  windows/amd64
  windows/arm64
  darwin/amd64
  darwin/arm64
)

for t in "${targets[@]}"; do
  GOOS="${t%/*}"
  GOARCH="${t#*/}"
  exe=""
  [[ "$GOOS" == "windows" ]] && exe=".exe"

  server="dnsloghub-${GOOS}-${GOARCH}${exe}"
  inv="invitegen-${GOOS}-${GOARCH}${exe}"

  echo ">> 构建 ${GOOS}/${GOARCH}"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$DIST/$server" ./cmd/server
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$DIST/$inv" ./cmd/invitegen

  stage="$DIST/.stage-${GOOS}-${GOARCH}"
  mkdir -p "$stage"
  cp "$DIST/$server" "$DIST/$inv" config.yaml "$stage"/
  if [[ -d scripts ]]; then
    cp -r scripts "$stage/"
    chmod +x "$stage"/scripts/*.sh 2>/dev/null || true
  fi
  archive="dnsloghub-${GOOS}-${GOARCH}.tar.gz"
  tar -czf "$DIST/$archive" -C "$stage" .
  rm -rf "$stage"

  sum="$(sha256 "$DIST/$archive")"
  echo "${sum}  ${archive}" > "$DIST/${archive}.sha256"
done

echo ""
echo "构建完成："
ls -1 "$DIST"
