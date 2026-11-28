# FreeLLMAPI

Repositorio: github.com/tashfeenahmed/freellmapi (licencia MIT)

Junta los planes gratis de muchos proveedores (Groq, Gemini, Mistral,
OpenRouter, Cerebras…) detrás de un endpoint /v1 compatible con OpenAI.
Elige el mejor modelo disponible, salta al siguiente cuando uno llega a su
límite y lleva la cuenta de lo que gastas. Infinito lo usa como motor del
chat, de las imágenes, del vídeo y de la voz.

1. Con la versión completa de Infinito se instala y arranca solo. Si no,
   pestaña Inicio, Instalar y Arrancar.
2. Abre el panel (Inicio → Panel web, http://localhost:3001) y, en Keys,
   añade las claves gratis de los proveedores que quieras.
3. Ordena la cadena de respaldo en Fallback Chain: arriba los modelos que
   prefieres; cuando uno se agota, pasa al siguiente.
4. La clave única de FreeLLMAPI la copia Infinito solo en Ajustes. Sirve
   con cualquier herramienta compatible con OpenAI apuntando a
   http://localhost:3001/v1.

Si ya tienes FreeLLMAPI en otro sitio (por ejemplo la app de escritorio,
que escucha en http://127.0.0.1:31415), cambia la URL en Ajustes y apaga
"Arrancar FreeLLMAPI al abrir".
