# Notas para Android (Termux), Linux y Windows

## Android (Termux)

    chmod +x infinito-android-termux-arm64
    ./infinito-android-termux-arm64

No hace falta pkg install de nada: el binario trae el Node.js de Termux y
sus bibliotecas en una carpeta propia, sin tocar los paquetes que ya
tengas. Toca la pantalla para pulsar botones y desliza para desplazarte.

Para dejarlo funcionando con la pantalla apagada:

    termux-wake-lock
    ./infinito-android-termux-arm64 serve omniroute

El soporte de Android es experimental, como en los propios repos.

## Linux

    chmod +x infinito-linux-x64      (o infinito-linux-arm64)
    ./infinito-linux-x64

## Windows

Abre infinito-win-x64.exe desde Windows Terminal o PowerShell. Si
SmartScreen avisa, pulsa "Más información" y "Ejecutar de todas formas": el
binario no está firmado.

## Claude Code

Claude Code no va incluido (es software de Anthropic con su propia
licencia). Instálalo una vez con:

    npm install -g @anthropic-ai/claude-code

## Teclado

    Tab / Mayús+Tab   cambiar de pestaña (o teclas 1-7 fuera del chat)
    ↑ ↓               moverse        Enter   elegir / enviar
    Esc               cancelar        Ctrl+C  salir
