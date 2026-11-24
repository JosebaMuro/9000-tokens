#!/usr/bin/env bash
# Instala el APK, lo abre pidiendo que arranque los dos repos y comprueba
# que responden con la clave que Infinito les da.
set -x
adb install -r -g "$1" || exit 1
adb shell am start -W -n app.infinito.tui/.MainActivity --es autostart omniroute,freellmapi
adb forward tcp:18686 tcp:8686; adb forward tcp:20128 tcp:20128; adb forward tcp:13001 tcp:3001
RA() { adb shell "run-as app.infinito.tui sh -c 'cd /data/data/app.infinito.tui/files && $1'"; }
ok=0
for i in $(seq 1 40); do
  sleep 15
  C=$(RA "cat config/config.json" 2>/dev/null)
  OK=$(echo "$C" | grep -o 'omr-[0-9a-f]*' | head -1); FK=$(echo "$C" | grep -o 'freellmapi-[0-9a-f]*' | head -1)
  o=$(curl -s -m 10 -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $OK" http://127.0.0.1:20128/v1/models)
  f=$(curl -s -m 10 -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $FK" http://127.0.0.1:13001/v1/models)
  echo "intento $i: omniroute=$o freellmapi=$f"
  RA "ls infinito/logs 2>/dev/null; tail -2 infinito/logs/omniroute.log infinito/logs/freellmapi.log 2>/dev/null"
  [ "$o" = 200 ] && [ "$f" = 200 ] && { ok=1; break; }
done
adb exec-out screencap -p > /dev/null
RA "for f in infinito/logs/*.log; do echo \"--- \$f\"; tail -80 \$f; done"
adb logcat -d | grep -E "Infinito|libinfinito|node|avc: denied|SIGSYS|seccomp" | tail -80
[ $ok = 1 ]
