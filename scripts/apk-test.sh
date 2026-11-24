#!/usr/bin/env bash
# Instala el APK en el emulador, lo abre y comprueba que Infinito arranca.
set -x
APK="$1"
mkdir -p shots
adb install -r -g "$APK"
adb shell am start -W -n app.infinito.tui/.MainActivity
sleep 20
adb exec-out screencap -p > shots/1-inicio.png
adb shell ps -A | grep -i infinito | tee shots/procesos.txt
adb forward tcp:18686 tcp:8686
H=$(curl -s -m 5 http://127.0.0.1:18686/health || true)
echo "health: $H" | tee shots/health.txt
# Toca la pestaña "Chat" (segunda pestaña, arriba a la izquierda).
SIZE=$(adb shell wm size | grep -o '[0-9]*x[0-9]*' | tail -1)
W=${SIZE%x*}; HH=${SIZE#*x}
adb shell input tap $((W * 30 / 100)) $((HH * 6 / 100))
sleep 3
adb exec-out screencap -p > shots/2-toque.png
adb shell input keyevent KEYCODE_BACK
sleep 2
adb exec-out screencap -p > shots/3-atras.png
adb logcat -d -s Infinito:* AndroidRuntime:E > shots/logcat.txt || true
echo "$H" | grep -q '"ok":true'
