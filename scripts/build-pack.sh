#!/usr/bin/env bash
# Construye el binario completo (con paquete offline) para un destino.
# Uso: scripts/build-pack.sh <linux-x64|linux-arm64|win-x64|termux-arm64> [salida]
set -euo pipefail
TARGET="$1"
OUT="$(mkdir -p "${2:-dist}" && cd "${2:-dist}" && pwd)"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="${WORK:-$ROOT/work-$TARGET}"
PACK="$WORK/pack"
NODE_MAJOR="${NODE_MAJOR:-22}"
VERSION="${VERSION:-dev}"
PY=python3; command -v python3 >/dev/null 2>&1 || PY=python
rm -rf "$WORK"; mkdir -p "$PACK/runtime" "$PACK/apps"

echo "== Node.js $NODE_MAJOR para $TARGET"
NODE_VER=$(curl -fsSL https://nodejs.org/dist/index.json | $PY -c "import json,sys; print([r['version'] for r in json.load(sys.stdin) if r['version'].startswith('v$NODE_MAJOR.')][0])")
case "$TARGET" in
  linux-x64|linux-arm64)
    a="${TARGET#linux-}"
    curl -fsSL "https://nodejs.org/dist/$NODE_VER/node-$NODE_VER-linux-$a.tar.xz" | tar -xJ -C "$PACK/runtime"
    mv "$PACK/runtime/node-$NODE_VER-linux-$a" "$PACK/runtime/node"
    rm -rf "$PACK/runtime/node/include" "$PACK/runtime/node/share"
    GOOS=linux; GOARCH="${a/x64/amd64}"; EXE="" ;;
  win-x64)
    curl -fsSLo "$WORK/node.zip" "https://nodejs.org/dist/$NODE_VER/node-$NODE_VER-win-x64.zip"
    (cd "$PACK/runtime" && unzip -q "$WORK/node.zip")
    mv "$PACK/runtime/node-$NODE_VER-win-x64" "$PACK/runtime/node"
    GOOS=windows; GOARCH=amd64; EXE=".exe" ;;
  termux-arm64)
    $PY "$ROOT/scripts/termux_fetch.py" "$PACK/runtime/termux" nodejs-lts ca-certificates
    NODE_VER="termux-$(grep '^nodejs-lts ' "$PACK/runtime/termux/PACKAGES.txt" | cut -d' ' -f2)"
    GOOS=linux; GOARCH=arm64; EXE="" ;;
  *) echo "destino desconocido: $TARGET"; exit 2 ;;
esac

NPMFLAGS=(--no-audit --no-fund --loglevel=error)
if [ "$TARGET" = termux-arm64 ]; then NPMFLAGS+=(--os=android --cpu=arm64 --ignore-scripts); fi

echo "== OmniRoute"
mkdir -p "$PACK/apps/omniroute"
( cd "$PACK/apps/omniroute"
  echo '{"name":"omniroute-offline","private":true}' > package.json
  npm install "${NPMFLAGS[@]}" "omniroute@${OMNIROUTE_VERSION:-latest}" )
case "$TARGET" in win-*) POS=win32;; termux-*) POS=android;; *) POS=linux;; esac
PARCH=x64; case "$TARGET" in *arm64) PARCH=arm64;; esac
node "$ROOT/scripts/prune.cjs" "$PACK/apps/omniroute/node_modules" $POS $PARCH
OMNI_VER=$(node -p "require('$PACK/apps/omniroute/node_modules/omniroute/package.json').version" 2>/dev/null || echo "?")

echo "== FreeLLMAPI"
git clone --depth 1 ${FREELLMAPI_REF:+--branch "$FREELLMAPI_REF"} https://github.com/tashfeenahmed/freellmapi "$WORK/freellmapi"
( cd "$WORK/freellmapi"
  npm ci --no-audit --no-fund --loglevel=error
  npm run build
  # Solo las dependencias que usa el servidor (el panel web ya está compilado).
  rm -rf node_modules client/node_modules server/node_modules shared/node_modules cli/node_modules
  FLFLAGS=(--omit=dev --no-audit --no-fund --loglevel=error)
  if [ "$TARGET" = termux-arm64 ]; then FLFLAGS+=(--os=android --cpu=arm64 --ignore-scripts); fi
  npm ci "${FLFLAGS[@]}" -w server -w shared
  node "$ROOT/scripts/deref.cjs" "$PWD"
  if [ "$TARGET" = termux-arm64 ]; then rm -rf node_modules/better-sqlite3 node_modules/@img node_modules/sharp; fi
  rm -rf .git docs repo-assets desktop client/src client/node_modules examples cli/src node_modules/@freellmapi/server/node_modules )
FL_VER=$(node -p "require('$WORK/freellmapi/server/package.json').version")
mv "$WORK/freellmapi" "$PACK/apps/freellmapi"

cat > "$PACK/pack.json" <<JSON
{"target":"$TARGET","built":"$(date -u +%Y-%m-%dT%H:%M:%SZ)","versions":{"infinito":"$VERSION","node":"$NODE_VER","omniroute":"$OMNI_VER","freellmapi":"$FL_VER"}}
JSON
cat "$PACK/pack.json"

echo "== Binarios"
cd "$ROOT"
go build -trimpath -o "$WORK/host-infinito$( [ "$(go env GOOS)" = windows ] && echo .exe)" .
CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$WORK/infinito$EXE" .
NAME="infinito-$TARGET"
[ "$TARGET" = termux-arm64 ] && NAME="infinito-android-termux-arm64"
cp "$WORK/infinito$EXE" "$OUT/$NAME-ligero$EXE"
"$WORK/host-infinito"* pack "$PACK" "$WORK/infinito$EXE" "$OUT/$NAME$EXE"
ls -la "$OUT"
