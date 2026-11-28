#!/usr/bin/env bash
# Instala el paquete offline del binario completo, abre la GUI sin navegador
# y comprueba que FreeLLMAPI responde y que la GUI lo ve.
set -euo pipefail
DIST="$1"
BIN=$(ls "$DIST"/infinito-* | grep -v ligero | head -1)
export INFINITO_DATA="${RUNNER_TEMP:-/tmp}/infinito-data"
export INFINITO_HOME="${RUNNER_TEMP:-/tmp}/infinito-home"
export INFINITO_FILES="${RUNNER_TEMP:-/tmp}/infinito-files"
export INFINITO_GUI_TOKEN=prueba
rm -rf "$INFINITO_DATA" "$INFINITO_HOME"
"$BIN" version
"$BIN" install
"$BIN" gui --no-open > "${RUNNER_TEMP:-/tmp}/gui.log" 2>&1 &
PID=$!
ok=0
for i in $(seq 1 60); do
  sleep 3
  f=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3001/v1/models || true)
  st=$(curl -s -H 'X-Token: prueba' http://127.0.0.1:8686/app/api/state || true)
  echo "intento $i: freellmapi=$f gui=$st"
  if echo "$st" | grep -q '"ok":true'; then ok=1; break; fi
done
code=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8686/app/)
echo "página de la GUI: $code"; [ "$code" = 200 ] || ok=0
curl -s -m 60 -N -H 'X-Token: prueba' -X POST -d '{"text":"Hola"}' http://127.0.0.1:8686/app/api/chat | head -5 || true
kill $PID 2>/dev/null || true
sleep 2
tail -20 "${RUNNER_TEMP:-/tmp}/gui.log"
for l in "$INFINITO_DATA"/logs/*.log; do echo "--- $l"; tail -15 "$l"; done
[ $ok = 1 ] || { echo "FALLO: FreeLLMAPI o la GUI no respondieron"; exit 1; }
echo "OK: FreeLLMAPI y la GUI responden"
