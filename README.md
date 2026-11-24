# 9000 tokens · Infinito

Interfaz de terminal (TUI) para usar **OmniRoute** y **FreeLLMAPI**, los dos
repos que juntan los planes gratis de decenas de proveedores de IA detrás de
un solo endpoint, en Windows, Linux y Android (Termux).

- **Sin internet para instalar**: la versión completa lleva dentro Node.js,
  OmniRoute y FreeLLMAPI ya instalados. Un clic en *Instalar todo* y listo.
- **Eliges cuál lanzar**: arranca, para y cambia entre OmniRoute, FreeLLMAPI
  y la pasarela integrada desde la pestaña Inicio.
- **Chat sin perder la conversación**: cambia de pasarela en mitad de una
  conversación y la siguiente respuesta la da la nueva. La conversación se
  guarda en disco.
- **Pasarela integrada en Go**: compatible con la API de OpenAI y con la de
  Anthropic, con cadena de respaldo entre 15 proveedores gratis (Groq,
  Gemini, Cerebras, OpenRouter, Mistral, NVIDIA…). Sirve a Claude Code
  directamente.
- **Ratón, panel táctil y pantalla táctil**: todas las pestañas, botones y
  listas se pueden pulsar. También funciona con teclado.
- **Lanza Claude Code** conectado a la pasarela que elijas.

Recursos originales: <https://antonspain-ia-recursos.surge.sh/infinito/>

## Descargas

En [Releases](../../releases) hay dos versiones por sistema:

| Sistema | Completa (offline, 280–400 MB) | Ligera (~10 MB) |
| --- | --- | --- |
| **Android (app, sin Termux)** | `infinito-android-arm64.apk` | |
| Windows x64 | `infinito-win-x64.exe` | `infinito-win-x64-ligero.exe` |
| Linux x64 | `infinito-linux-x64` | `infinito-linux-x64-ligero` |
| Linux ARM64 | `infinito-linux-arm64` | `infinito-linux-arm64-ligero` |
| Android (Termux) | `infinito-android-termux-arm64` | `infinito-android-termux-arm64-ligero` |

En Android, lo más fácil es el APK: se instala como cualquier app, trae su
propia terminal con teclas extra (Esc, Tab, Ctrl, flechas, zoom) y sigue
funcionando en segundo plano. Se instala fuera de Google Play.

```bash
chmod +x infinito-linux-x64
./infinito-linux-x64
```

## Uso

```
infinito                               interfaz
infinito install                       instala el paquete offline sin abrir la interfaz
infinito serve [omniroute] [freellmapi] pasarela integrada (y los repos) en segundo plano
infinito claude                        Claude Code conectado a la pasarela integrada
infinito env                           variables para conectar Claude Code a mano
```

Pasarela integrada: `http://127.0.0.1:8686/v1` (OpenAI) y
`http://127.0.0.1:8686` (Anthropic), modelo `auto`, clave en Ajustes.

Las guías completas están dentro del programa, en la pestaña **Guías**, y en
la carpeta [`guias/`](guias/).

## Cómo está hecho

- Un solo binario en Go (estático, sin dependencias) por plataforma. La
  interfaz usa Bubble Tea.
- La versión completa es ese binario con un paquete `tar.zst` pegado al
  final. Al instalar se descomprime en `~/.infinito` (Windows:
  `%LOCALAPPDATA%\infinito`).
- En Android el paquete lleva el Node.js de Termux y sus bibliotecas en una
  carpeta propia, sin tocar los paquetes del sistema.
- La app de Android (carpeta [`android/`](android/)) usa la terminal de
  Termux (Apache 2.0) y lleva el binario de Termux como librería nativa.
  Apunta a la API 28 para poder ejecutar Node.js desde su carpeta, como
  Termux; por eso no puede publicarse en Google Play.
- El flujo [`release.yml`](.github/workflows/release.yml) compila cada
  paquete en su propio sistema (Windows, Linux x64, Linux ARM64), lo prueba
  arrancando OmniRoute y FreeLLMAPI, y publica el release al subir una
  etiqueta `v*`.

Compilar a mano:

```bash
go build -o infinito .                      # versión ligera
scripts/build-pack.sh linux-x64 dist        # versión completa
```

## Licencias

Código de este repo: MIT. El software de terceros que va dentro de la
versión completa está en [THIRD_PARTY.md](THIRD_PARTY.md). Los tokens gratis
vienen de los planes de cada proveedor, con sus propios límites y
condiciones.
