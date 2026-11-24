Infinito: TUI para OmniRoute y FreeLLMAPI que funciona sin internet.

## Qué descargar

| Sistema | Versión completa (offline) | Versión ligera |
| --- | --- | --- |
| Windows x64 | `infinito-win-x64.exe` | `infinito-win-x64-ligero.exe` |
| Linux x64 | `infinito-linux-x64` | `infinito-linux-x64-ligero` |
| Linux ARM64 | `infinito-linux-arm64` | `infinito-linux-arm64-ligero` |
| Android (Termux) | `infinito-android-termux-arm64` | `infinito-android-termux-arm64-ligero` |

- **Completa**: lleva dentro Node.js, OmniRoute y FreeLLMAPI ya instalados.
  Pulsa *Instalar todo* en la pestaña Inicio y funcionan sin conexión.
- **Ligera**: solo la interfaz y la pasarela integrada (unos 10 MB). Sirve
  para usar la pasarela integrada o un OmniRoute/FreeLLMAPI que ya tengas.

En Linux y Termux: `chmod +x infinito-*` y ejecútalo. En Windows, ábrelo
desde Windows Terminal. Comprueba la descarga con `SHA256SUMS.txt`.

El soporte de Android es experimental, igual que en los propios repos.
Claude Code no va incluido: instálalo con `npm install -g @anthropic-ai/claude-code`.
