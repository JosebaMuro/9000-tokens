#!/usr/bin/env bash
# Prueba OmniRoute y FreeLLMAPI dentro del APK (arm64 por traducción en el emulador).
set -x
APK="$1"
adb shell getprop ro.product.cpu.abilist
adb install -r -g "$APK" || exit 1
adb shell am start -W -n app.infinito.tui/.MainActivity
sleep 10
DIR=$(adb shell pm path app.infinito.tui | sed 's/package://;s/base.apk//' | tr -d '\r')
LIB=$(adb shell "ls -d ${DIR}lib/*/libinfinito.so" | tr -d '\r')
echo "LIB=$LIB"
F=/data/data/app.infinito.tui/files
ENVS="HOME=$F/home TMPDIR=/data/data/app.infinito.tui/cache INFINITO_HOME=$F/config INFINITO_DATA=$F/infinito INFINITO_ANDROID_APP=1 PATH=/system/bin:/system/xbin"
RA() { adb shell "run-as app.infinito.tui sh -c 'cd $F && $1'"; }
time RA "env $ENVS $LIB install 2>&1 | tail -5"
RA "ls infinito infinito/runtime/termux/usr/bin | head -30; cat infinito/logs/postinstall.log | tail -20"
RA "env $ENVS LD_LIBRARY_PATH=$F/infinito/runtime/termux/usr/lib $F/infinito/runtime/termux/usr/bin/node -e \"console.log(process.version, process.platform, process.arch)\" 2>&1"
# Arranca los dos repos sin interfaz, en segundo plano, como haría la app.
RA "env $ENVS nohup $LIB serve omniroute freellmapi > $F/serve.log 2>&1 &"
for i in $(seq 1 24); do sleep 10; RA "cat $F/serve.log | tail -3"; done
adb forward tcp:18686 tcp:8686; adb forward tcp:20128 tcp:20128; adb forward tcp:13001 tcp:3001
echo "== pasarela"; curl -s -m 10 http://127.0.0.1:18686/health; echo
echo "== omniroute"; curl -s -m 20 -o /dev/null -w "%{http_code}\n" http://127.0.0.1:20128/
curl -s -m 20 http://127.0.0.1:20128/v1/models | head -c 400; echo
echo "== freellmapi"; curl -s -m 20 -o /dev/null -w "%{http_code}\n" http://127.0.0.1:13001/
curl -s -m 20 http://127.0.0.1:13001/v1/models | head -c 400; echo
echo "== registros"
RA "cat $F/serve.log | tail -40"
RA "for f in infinito/logs/*.log; do echo \"--- \$f\"; tail -60 \$f; done"
RA "ps -A -o PID,NAME,ARGS | grep -v ' \\[' "
true
