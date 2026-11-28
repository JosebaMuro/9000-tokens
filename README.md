# 9000 tokens · Infinito

Chat de IA gratis que usa **FreeLLMAPI** como motor (el repo que junta los
planes gratis de decenas de proveedores detrás de un solo endpoint), con
interfaz gráfica (GUI) y de terminal (TUI), en Windows, Linux y Android.

- **Sin internet para instalar**: la versión completa lleva dentro Node.js y
  FreeLLMAPI. La primera vez se instala y arranca solo.
- **GUI o TUI al abrir**: la GUI es una app visual estilo Android moderno (en
  el PC se abre en el navegador; en el APK, a pantalla completa). La TUI es
  la de terminal. Las dos comparten la misma conversación.
- **Archivos y multimedia**: desde el chat creas imágenes, vídeo y voz con
  FreeLLMAPI, guardas respuestas y su código en archivos y exportas la
  conversación. Todo queda en la pestaña Archivos.
- **Ratón, panel táctil y pantalla táctil** en la TUI; también teclado.

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

En Android, lo más fácil es el APK: se instala como cualquier app, al abrirlo
eliges la app visual o la terminal (con teclas extra: Esc, Tab, Ctrl,
flechas, zoom) y sigue funcionando en segundo plano. Se instala fuera de
Google Play.

```bash
chmod +x infinito-linux-x64
./infinito-linux-x64
```

## Uso

```
infinito                 pregunta si abrir la GUI o la TUI
infinito gui [--no-open] interfaz gráfica (http://127.0.0.1:8686/app/)
infinito tui             interfaz de terminal
infinito serve           solo FreeLLMAPI, para usar su API en :3001
infinito install         instala el paquete offline sin abrir la interfaz
```

Los archivos que crea el chat se guardan en la carpeta `Infinito` de tu
usuario (`Documentos\Infinito` en Windows).

Las guías completas están dentro del programa, en la pestaña **Guías**, y en
la carpeta [`guias/`](guias/).

## Cómo está hecho

- Un solo binario en Go (estático, sin dependencias) por plataforma. La TUI
  usa Bubble Tea; la GUI es una página web incluida en el binario que sirve
  él mismo solo en 127.0.0.1, protegida con una clave por sesión.
- La versión completa es ese binario con un paquete `tar.zst` pegado al
  final. Al instalar se descomprime en `~/.infinito` (Windows:
  `%LOCALAPPDATA%\infinito`).
- En Android el paquete lleva el Node.js de Termux y sus bibliotecas en una
  carpeta propia, sin tocar los paquetes del sistema.
- La app de Android (carpeta [`android/`](android/)) tiene una pantalla de
  elección nativa, muestra la GUI en un WebView y la TUI con la terminal de
  Termux (Apache 2.0). Lleva el binario de Termux como librería nativa.
  Apunta a la API 28 para poder ejecutar Node.js desde su carpeta, como
  Termux; por eso no puede publicarse en Google Play.
- El flujo [`release.yml`](.github/workflows/release.yml) compila cada
  paquete en su propio sistema (Windows, Linux x64, Linux ARM64), lo prueba
  arrancando FreeLLMAPI y la GUI, y publica el release al subir una
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
