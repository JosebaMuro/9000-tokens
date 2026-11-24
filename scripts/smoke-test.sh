#!/usr/bin/env bash
# Instala el paquete offline del binario completo y arranca OmniRoute y
# FreeLLMAPI para comprobar que responden.
set -euo pipefail
DIST="$1"
BIN=$(ls "$DIST"/infinito-* | grep -v ligero | head -1)
export INFINITO_DATA="${RUNNER_TEMP:-/tmp}/infinito-data"
export INFINITO_HOME="${RUNNER_TEMP:-/tmp}/infinito-home"
rm -rf "$INFINITO_DATA" "$INFINITO_HOME"
"$BIN" version
"$BIN" install
"$BIN" serve omniroute freellmapi > "${RUNNER_TEMP:-/tmp}/serve.log" 2>&1 &
PID=$!
ok=0
for i in $(seq 1 60); do
  sleep 3
  o=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:20128/v1/models || true)
  f=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3001/v1/models || true)
  g=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8686/health || true)
  echo "intento $i: omniroute=$o freellmapi=$f integrada=$g"
  if [ "$o" != 000 ] && [ "$f" != 000 ] && [ "$g" = 200 ]; then ok=1; break; fi
done
kill $PID 2>/dev/null || true
cat "${RUNNER_TEMP:-/tmp}/serve.log" | tail -20
for l in "$INFINITO_DATA"/logs/*.log; do echo "--- $l"; tail -15 "$l"; done
[ $ok = 1 ] || { echo "FALLO: alguna pasarela no respondió"; exit 1; }
echo "OK: las tres pasarelas responden"
