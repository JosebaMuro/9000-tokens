#!/usr/bin/env bash
# Instala el APK en el emulador, lo abre y comprueba que Infinito arranca.
set -x
APK="$1"
mkdir -p shots
adb install -r -g "$APK"
adb logcat -c
# Diagnóstico: el binario por sí solo, con el usuario de la app.
DIR=$(adb shell pm path app.infinito.tui | sed 's/package://;s/base.apk//' | tr -d '\r')
LIB="${DIR}lib/x86_64/libinfinito.so"
adb shell "ls -l $LIB"
adb shell "run-as app.infinito.tui $LIB version"; echo "salida: $?"
adb shell "run-as app.infinito.tui sh -c 'mkdir -p files/home files/config; cd files; HOME=\$PWD/home INFINITO_HOME=\$PWD/config INFINITO_DATA=\$PWD/infinito timeout 8 $LIB serve'" 2>&1 | head -20
adb shell am start -W -n app.infinito.tui/.MainActivity
sleep 20
adb exec-out screencap -p > shots/1-inicio.png
adb shell ps -A -o PID,PPID,USER,NAME,ARGS | grep -iE 'infinito|PID' | tee shots/procesos.txt
adb shell dumpsys activity services app.infinito.tui | head -40
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
adb logcat -d > shots/logcat.txt || true
echo "===== logcat (app) ====="
PIDS=$(adb shell pidof app.infinito.tui || true)
grep -E "app.infinito|Infinito|I Infinito|AndroidRuntime|FATAL|libinfinito|termux|DEBUG  " shots/logcat.txt | tail -150
echo "$H" | grep -q '"ok":true'
