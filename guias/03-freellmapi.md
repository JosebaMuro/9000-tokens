# FreeLLMAPI en 4 pasos

Repositorio: github.com/tashfeenahmed/freellmapi (licencia MIT)

Junta los planes gratis de 87 proveedores (Groq, Gemini, Mistral,
OpenRouter, Cerebras…) detrás de un endpoint /v1 compatible con OpenAI.
Elige el mejor modelo disponible, salta al siguiente cuando uno llega a su
límite y lleva la cuenta de lo que gastas.

1. Con la versión completa de Infinito: pestaña Inicio, Instalar y
   Arrancar. Si no, instálalo con la app de escritorio (Mac, Windows) desde la página de
   Releases del repo, o con Docker:

       curl -fsSL https://freellmapi.co/install.sh | bash

   En Android hay una guía experimental para Termux en el repo
   (docs/en/install/02-android-termux.md).
2. Abre el panel en http://localhost:3001 y, en Keys, añade las claves
   gratis de los proveedores que quieras.
   La app de escritorio escucha en http://127.0.0.1:31415: si usas la app,
   cambia la URL en Infinito, pestaña Ajustes.
3. Ordena la cadena de respaldo en Fallback Chain: arriba los modelos que
   prefieres; cuando uno se agota, pasa al siguiente.
4. Copia tu clave única en la cabecera de Keys y pégala en Infinito,
   pestaña Ajustes, campo "Clave FreeLLMAPI". Sirve con cualquier
   herramienta compatible con OpenAI apuntando a http://localhost:3001/v1.

Claude Code con FreeLLMAPI (lo hace Infinito por ti en la pestaña Claude
Code):

    ANTHROPIC_BASE_URL=http://localhost:3001
    ANTHROPIC_AUTH_TOKEN=<clave única>
    CLAUDE_CODE_AUTO_MODE_SERVER=0
    claude
