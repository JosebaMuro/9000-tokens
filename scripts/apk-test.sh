#!/usr/bin/env bash
# Instala el APK en el emulador y prueba la pantalla de elección, la GUI
# (WebView con el servidor local) y el cambio a la TUI.
set -x
APK="$1"
mkdir -p shots
adb install -r -g "$APK"
adb logcat -c
DIR=$(adb shell pm path app.infinito.tui | sed 's/package://;s/base.apk//' | tr -d '\r')
LIB="${DIR}lib/x86_64/libinfinito.so"
adb shell "run-as app.infinito.tui $LIB version"; echo "salida: $?"

. "$(dirname "$0")/android-lib.sh"

ok=1
adb shell am start -W -n app.infinito.tui/.ChooserActivity
sleep 4
adb exec-out screencap -p > shots/1-eleccion.png
texts | tee shots/eleccion.txt
grep -q "Aplicación" shots/eleccion.txt || { echo "FALLO: no sale la pantalla de elección"; ok=0; }

tap_text "Aplicación"
sleep 20
adb exec-out screencap -p > shots/2-gui.png
adb shell ps -A -o PID,NAME,ARGS | grep -i infinito | grep -v app.infinito | tee shots/procesos-gui.txt
adb forward tcp:18686 tcp:8686
code=$(curl -s -m 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:18686/app/)
echo "GUI /app/: $code"; [ "$code" = 200 ] || ok=0
PID=$(adb shell ps -A -o PID,ARGS | grep -E 'infinito (gui|tui)' | head -1 | awk '{print $1}')
TOK=$(adb shell "run-as app.infinito.tui cat /proc/$PID/environ" | tr '\0' '\n' | grep INFINITO_GUI_TOKEN | cut -d= -f2 | tr -d '\r')
ST=$(curl -s -m 5 -H "X-Token: $TOK" http://127.0.0.1:18686/app/api/state)
echo "estado: $ST" | tee shots/estado.txt
echo "$ST" | grep -q '"version"' || ok=0
texts | tee shots/gui.txt
adb logcat -d | grep -E "chromium.*CONSOLE|Uncaught" | tee shots/consola.txt
grep -q "Uncaught" shots/consola.txt && { echo "FALLO: error de JavaScript en la GUI"; ok=0; }
adb logcat -d | grep -q "app/#t=" || { echo "FALLO: el WebView no cargó la GUI"; ok=0; }

# Cambiar a la TUI desde la pantalla de elección.
adb shell am start -W -n app.infinito.tui/.ChooserActivity --ez choose true
sleep 3
tap_text "Terminal"
sleep 12
adb exec-out screencap -p > shots/3-tui.png
adb shell ps -A -o PID,NAME,ARGS | grep -i infinito | grep -v app.infinito | tee shots/procesos-tui.txt
grep -q " tui" shots/procesos-tui.txt || { echo "FALLO: no arrancó la TUI"; ok=0; }
grep -q " gui" shots/procesos-tui.txt && { echo "FALLO: la GUI sigue en marcha"; ok=0; }
code=$(curl -s -m 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:18686/app/)
echo "GUI tras cambiar: $code"

adb logcat -d > shots/logcat.txt || true
grep -E "app.infinito|Infinito|AndroidRuntime|FATAL|libinfinito|chromium" shots/logcat.txt | tail -120
[ $ok = 1 ]
