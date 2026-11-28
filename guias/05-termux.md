# Notas para Android (Termux), Linux y Windows

## Android con la app (lo más fácil)

Descarga infinito-android-arm64.apk en el móvil y ábrelo. Android pedirá
permiso para instalar apps de fuera de Google Play. Al abrir la app eliges:

- Aplicación: el chat visual a pantalla completa. Para pasar a la
  terminal: Ajustes → Cambiar a terminal.
- Terminal: la TUI, con una fila de teclas abajo (▣ GUI · ⌨ teclado ·
  Esc · Tab · Ctrl · Alt · flechas · Pág↑/↓ · A+/A− · Pegar). Pellizca
  para cambiar el tamaño de letra.

Lo que generes se ve en Archivos; con el botón de descarga se guarda una
copia en Descargas/Infinito. FreeLLMAPI sigue funcionando aunque cambies
de app (verás una notificación "Infinito en marcha"); pulsa "Salir" en la
notificación para cerrarlo todo.

## Android (Termux)

    chmod +x infinito-android-termux-arm64
    ./infinito-android-termux-arm64

Elige Terminal, o Aplicación para abrir la GUI en el navegador del móvil.
No hace falta pkg install de nada: el binario trae el Node.js de Termux y
sus bibliotecas en una carpeta propia, sin tocar los paquetes que ya
tengas. Toca la pantalla para pulsar botones y desliza para desplazarte.

Para dejarlo funcionando con la pantalla apagada:

    termux-wake-lock
    ./infinito-android-termux-arm64 serve

El soporte de Android es experimental, como en los propios repos.

## Linux

    chmod +x infinito-linux-x64      (o infinito-linux-arm64)
    ./infinito-linux-x64

## Windows

Abre infinito-win-x64.exe desde Windows Terminal o PowerShell. Si
SmartScreen avisa, pulsa "Más información" y "Ejecutar de todas formas": el
binario no está firmado.

## Teclado

    Tab / Mayús+Tab   cambiar de pestaña (o teclas 1-6 fuera del chat)
    ↑ ↓               moverse        Enter   elegir / enviar
    Esc               cancelar        Ctrl+C  salir
