# Infinito: cómo funciona

Infinito es un chat de IA gratis que usa FreeLLMAPI como motor. Es un
único programa que no necesita internet para instalarse: la versión
completa lleva dentro Node.js y FreeLLMAPI listos para descomprimir.

Tiene dos interfaces con la misma conversación:

- Aplicación (GUI): ventana visual estilo app de móvil. En el PC se abre en
  el navegador; en la app de Android, a pantalla completa.
- Terminal (TUI): todo dentro de la terminal, con ratón, dedo o teclado.

Al abrir Infinito eliges una. Si marcas "Recordar mi elección", la próxima
vez entra directamente (se cambia en Ajustes).

## Primeros pasos

1. La primera vez, Infinito instala y arranca FreeLLMAPI solo. Lo ves en
   la pestaña Inicio.
2. Escribe en el Chat. Con el modelo "auto", FreeLLMAPI elige el mejor
   proveedor gratuito que esté libre.
3. Si no responde ningún proveedor, abre el panel web de FreeLLMAPI
   (Inicio → Panel web) y añade una clave gratuita (guía "Claves gratis").

## Archivos y multimedia

Desde el chat puedes:

- Crear imágenes, vídeo o voz (si FreeLLMAPI tiene un proveedor que lo
  haga): en la GUI con el botón +, en la TUI con los botones o con
  /imagen, /video y /voz seguidos de la descripción.
- Guardar la última respuesta (.md), su código (cada bloque en su archivo,
  .py, .js, .html…) o la conversación entera.

Todo queda en la pestaña Archivos y en la carpeta Infinito de tu usuario
(Documentos\Infinito en Windows, Descargas/Infinito en Termux).

## Sin interfaz

    infinito serve          solo FreeLLMAPI, para usar su API (puerto 3001)
    infinito gui --no-open  la GUI sin abrir el navegador

## Dónde se guarda todo

    Configuración:           ~/.config/infinito/config.json
                             (Windows: %APPDATA%\infinito\config.json)
    Conversación del chat:   la misma carpeta, chat.json
    Programas instalados:    ~/.infinito  (Windows: %LOCALAPPDATA%\infinito)

Puedes cambiar las carpetas con INFINITO_HOME, INFINITO_DATA e
INFINITO_FILES.
