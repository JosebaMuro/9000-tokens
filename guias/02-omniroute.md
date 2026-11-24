# OmniRoute + Claude Code en 5 pasos

Repositorio: github.com/cheaper-inference/OmniRoute (licencia MIT)

Pasarela de IA con un endpoint local, más de 150 proveedores con plan
gratis y cambio automático cuando uno se agota. Comprime el contexto para
gastar entre un 15 y un 95 % menos de tokens y lanza Claude Code ya
conectado con un solo comando.

1. Instala Node.js desde nodejs.org (versión LTS). Lo necesitas para npm.
   En Termux: pkg install nodejs
2. Instala y arranca OmniRoute en la terminal:

       npm install -g omniroute
       omniroute

   Se abre el panel en http://localhost:20128
3. Conecta tus proveedores gratis en el panel: Providers, elige uno (Groq,
   Gemini, Mistral, OpenRouter…) y pega su clave. Cuantos más conectes, más
   tokens al mes.
4. Copia tu clave de OmniRoute en Dashboard → Endpoints. Tu endpoint es
   http://localhost:20128/v1 y el modelo puede ser auto.
   Pégala en Infinito, pestaña Ajustes, campo "Clave OmniRoute".
5. Abre Claude Code a través de OmniRoute en la carpeta de tu proyecto:

       omniroute run claude --model auto

   O desde Infinito: pestaña Claude Code → "Claude Code vía OmniRoute".

Comprobar que funciona:

    curl http://localhost:20128/v1/models -H "Authorization: Bearer TU_CLAVE"

Con la versión completa de Infinito no necesitas npm: pulsa Instalar y
Arrancar en la pestaña Inicio. Infinito también detecta un OmniRoute que
ya tengas en marcha.
