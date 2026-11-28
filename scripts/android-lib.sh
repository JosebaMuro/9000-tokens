# Funciones comunes para las pruebas en el emulador.
mkdir -p shots
# Toca el elemento de la pantalla que contiene un texto.
tap_text() {
  adb shell uiautomator dump /sdcard/ui.xml >/dev/null
  adb shell cat /sdcard/ui.xml > shots/ui.xml
  b=$(python3 - "$1" <<'PY'
import re,sys
x=open('shots/ui.xml',encoding='utf-8',errors='ignore').read()
for m in re.finditer(r'<node [^>]*>',x):
    n=m.group(0)
    if sys.argv[1] in n:
        b=re.search(r'bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"',n)
        if b:
            a=list(map(int,b.groups())); print((a[0]+a[2])//2,(a[1]+a[3])//2); break
PY
)
  echo "tocar '$1' en: $b"
  [ -n "$b" ] && adb shell input tap $b
}
texts() { adb shell uiautomator dump /sdcard/ui.xml >/dev/null; adb shell cat /sdcard/ui.xml | grep -o 'text="[^"]*"' | grep -v 'text=""' | head -60; }

