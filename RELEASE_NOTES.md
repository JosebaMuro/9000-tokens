Infinito: chat de IA gratis con FreeLLMAPI, con interfaz gráfica (GUI) y de terminal (TUI), que se instala sin internet.

Novedades de la 1.2.0:

- Al abrir eliges la GUI (estilo app Android moderna) o la TUI. Comparten la conversación.
- El chat usa FreeLLMAPI. Desde el chat creas imágenes, vídeo y voz, guardas respuestas y código en archivos y exportas la conversación. Nueva pestaña Archivos.
- Se quitan OmniRoute, la pasarela integrada y el lanzador de Claude Code.

## Qué descargar

| Sistema | Versión completa (offline) | Versión ligera |
| --- | --- | --- |
| **Android (app)** | `infinito-android-arm64.apk` | |
| Windows x64 | `infinito-win-x64.exe` | `infinito-win-x64-ligero.exe` |
| Linux x64 | `infinito-linux-x64` | `infinito-linux-x64-ligero` |
| Linux ARM64 | `infinito-linux-arm64` | `infinito-linux-arm64-ligero` |
| Android (Termux) | `infinito-android-termux-arm64` | `infinito-android-termux-arm64-ligero` |

- **Completa**: lleva dentro Node.js y FreeLLMAPI. La primera vez se
  instalan y arrancan solos, sin conexión.
- **Ligera**: solo Infinito (unos 10 MB), para usar un FreeLLMAPI que ya
  tengas (cambia la URL en Ajustes).

**Android, lo más fácil**: descarga `infinito-android-arm64.apk` en el móvil
y ábrelo (Android te pedirá permitir instalar apps de esta fuente). Al abrirla
eliges la app visual o la terminal: no hace falta Termux. Necesita unos 2 GB
libres. No está en Google Play.

En Linux y Termux: `chmod +x infinito-*` y ejecútalo. En Windows, ábrelo
desde Windows Terminal. Comprueba la descarga con `SHA256SUMS.txt`.

El soporte de Android es experimental, igual que en los propios repos.
