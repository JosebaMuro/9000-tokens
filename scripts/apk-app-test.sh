#!/usr/bin/env bash
# Instala el APK (con el paquete offline), abre la GUI y comprueba que
# FreeLLMAPI arranca dentro de la app y que el chat le llega.
set -x
adb install -r -g "$1" || exit 1
RA() { adb shell "run-as app.infinito.tui sh -c 'cd /data/data/app.infinito.tui && $1'"; }
# Recordar la GUI para que se abra directamente.
RA "mkdir -p shared_prefs && echo \"<?xml version='1.0' encoding='utf-8' standalone='yes' ?><map><string name=\\\"start\\\">gui</string></map>\" > shared_prefs/modo.xml"
adb shell am start -W -n app.infinito.tui/.ChooserActivity
adb forward tcp:18686 tcp:8686; adb forward tcp:13001 tcp:3001
ok=0
for i in $(seq 1 40); do
  sleep 15
  PID=$(adb shell ps -A -o PID,ARGS | grep -E 'infinito (gui|tui)' | head -1 | awk '{print $1}')
  TOK=$(RA "cat /proc/$PID/environ" | tr '\0' '\n' | grep INFINITO_GUI_TOKEN | cut -d= -f2 | tr -d '\r')
  ST=$(curl -s -m 10 -H "X-Token: $TOK" http://127.0.0.1:18686/app/api/state)
  echo "intento $i: $ST"
  RA "tail -2 files/infinito/logs/freellmapi.log 2>/dev/null"
  echo "$ST" | grep -q '"ok":true' && { ok=1; break; }
done
curl -s -m 60 -N -H "X-Token: $TOK" -X POST -d '{"text":"Hola"}' http://127.0.0.1:18686/app/api/chat | head -5
curl -s -m 10 -H "X-Token: $TOK" http://127.0.0.1:18686/app/api/history | head -c 600; echo
adb shell uiautomator dump /sdcard/ui.xml >/dev/null; adb shell cat /sdcard/ui.xml | grep -o 'text="[^"]*"' | grep -v 'text=""' | head -40
RA "for f in files/infinito/logs/*.log; do echo \"--- \$f\"; tail -60 \$f; done"
adb logcat -d | grep -E "Infinito|libinfinito|node|avc: denied|SIGSYS|seccomp|AndroidRuntime" | tail -80
[ $ok = 1 ]
