# Infinito: cómo funciona

Infinito es un único programa que no necesita internet para instalarse.
La versión completa lleva dentro:

- Node.js, OmniRoute y FreeLLMAPI ya instalados, listos para descomprimir.
- Su propia pasarela integrada (puerto 8686), compatible con la API de
  OpenAI (/v1/chat/completions) y con la de Anthropic (/v1/messages). Junta
  los planes gratis de varios proveedores y, cuando uno llega a su límite,
  salta al siguiente.

Todo se maneja con el ratón, el panel táctil o el dedo (en Termux), o con
el teclado.

## Primeros pasos

1. Pestaña Inicio: pulsa "Instalar todo". Tarda uno o dos minutos y solo
   se hace una vez.
2. Elige qué pasarela quieres usar y pulsa "Arrancar":
   - Pasarela integrada: pon tus claves en la pestaña Proveedores.
   - OmniRoute o FreeLLMAPI: pulsa "Panel web" y añade allí tus claves.
     Luego copia su clave en la pestaña Ajustes.
3. Pulsa "Usar en el chat" en la que quieras y escribe en la pestaña Chat.
   Puedes cambiar de pasarela en mitad de la conversación con los botones
   de arriba del chat: no se pierde nada.
4. Pestaña Claude Code para abrir Claude Code conectado a la que elijas.

## Usar la pasarela integrada desde otras herramientas

    URL OpenAI:     http://127.0.0.1:8686/v1
    URL Anthropic:  http://127.0.0.1:8686
    Clave:          la de la pestaña Ajustes
    Modelo:         auto (o proveedor/modelo, p. ej. groq/llama-3.3-70b-versatile)

Sin interfaz (por ejemplo, para dejarlo en segundo plano):

    infinito serve                         solo la pasarela integrada
    infinito serve omniroute freellmapi    y además los dos repos

## Dónde se guarda todo

    Configuración y claves:  ~/.config/infinito/config.json
                             (Windows: %APPDATA%\infinito\config.json)
    Conversación del chat:   la misma carpeta, chat.json
    Programas instalados:    ~/.infinito  (Windows: %LOCALAPPDATA%\infinito)

Puedes cambiar las carpetas con INFINITO_HOME e INFINITO_DATA.
