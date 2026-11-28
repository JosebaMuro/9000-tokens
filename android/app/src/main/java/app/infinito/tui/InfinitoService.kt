package app.infinito.tui

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.ContentValues
import android.os.Environment
import android.os.StrictMode
import android.provider.MediaStore
import android.webkit.MimeTypeMap
import android.widget.Toast
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Binder
import android.os.Build
import android.os.FileObserver
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.PowerManager
import android.util.Log
import com.termux.terminal.TerminalSession
import com.termux.terminal.TerminalSessionClient
import java.io.File

/**
 * Mantiene Infinito (y FreeLLMAPI) vivo aunque la app pase a
 * segundo plano. La sesión de terminal vive aquí; la actividad solo la muestra.
 */
class InfinitoService : Service(), TerminalSessionClient {

    inner class LocalBinder : Binder() {
        val service: InfinitoService get() = this@InfinitoService
    }

    private val binder = LocalBinder()
    private val main = Handler(Looper.getMainLooper())
    private var wakeLock: PowerManager.WakeLock? = null
    private var urlObserver: FileObserver? = null

    var session: TerminalSession? = null
        private set

    /** La actividad visible, para repintar y reenviar eventos. */
    var ui: TerminalSessionClient? = null

    override fun onBind(intent: Intent?): IBinder = binder

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopEverything()
            return START_NOT_STICKY
        }
        startForeground(NOTIF_ID, buildNotification())
        return START_NOT_STICKY
    }

    /** "gui" o "tui": qué está ejecutando la sesión actual. */
    var mode: String = ""
        private set

    /** Clave de la GUI local (solo esta app la conoce). */
    val guiToken: String = java.util.UUID.randomUUID().toString().replace("-", "")

    /** La GUI se entera aquí si Infinito se cierra (código de salida). */
    var guiListener: ((Int) -> Unit)? = null

    private var switching: (() -> Unit)? = null

    /** Carpeta donde Infinito guarda respuestas, código, imágenes, vídeo y voz. */
    fun filesFolder(): File =
        (getExternalFilesDir("archivos") ?: File(filesDir, "archivos")).apply { mkdirs() }

    /**
     * Da la sesión del modo pedido. Si estaba en marcha la del otro modo, la
     * cierra con SIGTERM (para que pare FreeLLMAPI) y luego arranca la nueva.
     */
    fun requestSession(want: String, ready: (TerminalSession) -> Unit) {
        val cur = session
        if (cur != null && cur.isRunning && mode == want) { ready(cur); return }
        if (cur != null && cur.isRunning) {
            switching = { ready(startSession(want)) }
            runCatching { android.system.Os.kill(cur.pid, android.system.OsConstants.SIGTERM) }
            main.postDelayed({
                if (switching != null) { cur.finishIfRunning() }
            }, 5000)
            return
        }
        ready(startSession(want))
    }

    private fun startSession(want: String): TerminalSession {
        mode = want
        val files = filesDir.absolutePath
        val home = File(filesDir, "home").apply { mkdirs() }
        val tmp = cacheDir.apply { mkdirs() }
        val openFile = File(filesDir, "open-url")
        val bin = File(applicationInfo.nativeLibraryDir, "libinfinito.so").absolutePath
        val env = listOf(
            "HOME=${home.absolutePath}",
            "TMPDIR=${tmp.absolutePath}",
            "TERM=xterm-256color",
            "COLORTERM=truecolor",
            "LANG=es_ES.UTF-8",
            "PATH=/system/bin:/system/xbin",
            "ANDROID_ROOT=" + (System.getenv("ANDROID_ROOT") ?: "/system"),
            "ANDROID_DATA=" + (System.getenv("ANDROID_DATA") ?: "/data"),
            "INFINITO_DATA=$files/infinito",
            "INFINITO_HOME=$files/config",
            "INFINITO_FILES=${filesFolder().absolutePath}",
            "INFINITO_GUI_TOKEN=$guiToken",
            "INFINITO_ANDROID_APP=1",
            "INFINITO_OPEN_FILE=${openFile.absolutePath}",
            "INFINITO_CRASH_FILE=${File(filesDir, "crash.txt").absolutePath}",
        ).toTypedArray()
        watchOpenUrl(openFile)
        val args = if (want == "gui") arrayOf("infinito", "gui", "--no-open") else arrayOf("infinito", "tui")
        val s = TerminalSession(bin, home.absolutePath, args, env, 3000, this)
        session = s
        // La GUI no tiene pantalla de terminal: le damos un tamaño para que arranque.
        if (want == "gui") s.updateSize(100, 40, 10, 20)
        acquireWakeLock()
        return s
    }

    /** Puerto de la GUI según la configuración de Infinito. */
    fun guiPort(): Int = runCatching {
        org.json.JSONObject(File(filesDir, "config/config.json").readText()).optInt("gui_port", 8686)
    }.getOrDefault(8686).let { if (it in 1..65535) it else 8686 }

    /** Infinito escribe aquí una URL o la ruta de un archivo que hay que abrir. */
    private fun watchOpenUrl(f: File) {
        urlObserver?.stopWatching()
        @Suppress("DEPRECATION")
        val obs = object : FileObserver(f.parent!!, CLOSE_WRITE) {
            override fun onEvent(event: Int, path: String?) {
                if (path != f.name) return
                val url = runCatching { f.readText().trim() }.getOrNull() ?: return
                f.delete()
                if (url.isEmpty()) return
                main.post { runCatching { if (url.startsWith("/")) openPath(File(url)) else openWeb(url) } }
            }
        }
        obs.startWatching()
        urlObserver = obs
    }

    private fun openWeb(url: String) {
        startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }

    private fun mimeOf(f: File): String =
        MimeTypeMap.getSingleton().getMimeTypeFromExtension(f.extension.lowercase()) ?: when (f.extension.lowercase()) {
            "md" -> "text/markdown"
            "go", "py", "js", "ts", "sh", "kt", "rs", "c", "cpp", "java" -> "text/plain"
            else -> "application/octet-stream"
        }

    /** Abre un archivo de Infinito con la app que toque (galería, reproductor…). */
    fun openPath(f: File) {
        if (f.isDirectory) {
            toast("Tus archivos están en Descargas/Infinito cuando los guardas, y en la pestaña Archivos.")
            return
        }
        val uri = if (Build.VERSION.SDK_INT >= 29) copyToDownloads(f) else {
            // Android 9 o anterior: el archivo ya está en almacenamiento compartido.
            StrictMode.setVmPolicy(StrictMode.VmPolicy.Builder().build())
            Uri.fromFile(f)
        }
        if (uri == null) { toast("No pude abrir " + f.name); return }
        val i = Intent(Intent.ACTION_VIEW).setDataAndType(uri, mimeOf(f))
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_GRANT_READ_URI_PERMISSION)
        runCatching { startActivity(Intent.createChooser(i, f.name).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
            .onFailure { toast("No hay ninguna app para abrir " + f.name) }
    }

    /** Guarda una copia en Descargas/Infinito. */
    fun saveToDownloads(f: File) {
        if (Build.VERSION.SDK_INT >= 29) {
            if (copyToDownloads(f) != null) toast("Guardado en Descargas/Infinito: " + f.name)
            else toast("No pude guardar " + f.name)
        } else {
            toast("Guardado en " + f.absolutePath)
        }
    }

    private val copied = HashMap<String, Uri>()

    private fun copyToDownloads(f: File): Uri? {
        if (Build.VERSION.SDK_INT < 29) return null
        copied[f.absolutePath]?.let { return it }
        return runCatching {
            val values = ContentValues().apply {
                put(MediaStore.MediaColumns.DISPLAY_NAME, f.name)
                put(MediaStore.MediaColumns.MIME_TYPE, mimeOf(f))
                put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS + "/Infinito")
            }
            val uri = contentResolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, values)!!
            contentResolver.openOutputStream(uri)!!.use { out -> f.inputStream().use { it.copyTo(out) } }
            copied[f.absolutePath] = uri
            uri
        }.onFailure { Log.e(TAG, "copia a Descargas", it) }.getOrNull()
    }

    private fun toast(s: String) = main.post { Toast.makeText(this, s, Toast.LENGTH_LONG).show() }

    private fun acquireWakeLock() {
        if (wakeLock != null) return
        val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "infinito:freellmapi").apply {
            setReferenceCounted(false)
            acquire()
        }
    }

    fun stopEverything() {
        session?.let { s -> if (s.isRunning) runCatching { android.system.Os.kill(s.pid, android.system.OsConstants.SIGTERM) } }
        session?.let { s -> main.postDelayed({ s.finishIfRunning() }, 3000) }
        session = null
        urlObserver?.stopWatching()
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
        stopForeground(true)
        stopSelf()
    }

    override fun onDestroy() {
        session?.let { s -> if (s.isRunning) runCatching { android.system.Os.kill(s.pid, android.system.OsConstants.SIGTERM) } }
        wakeLock?.let { if (it.isHeld) it.release() }
        super.onDestroy()
    }

    private fun buildNotification(): Notification {
        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (Build.VERSION.SDK_INT >= 26) {
            nm.createNotificationChannel(
                NotificationChannel(CHANNEL, "Infinito en marcha", NotificationManager.IMPORTANCE_LOW)
            )
        }
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, ChooserActivity::class.java).putExtra("resume", true),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val stop = PendingIntent.getService(
            this, 1, Intent(this, InfinitoService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val b = if (Build.VERSION.SDK_INT >= 26) Notification.Builder(this, CHANNEL) else
            @Suppress("DEPRECATION") Notification.Builder(this)
        return b.setContentTitle("Infinito en marcha")
            .setContentText("FreeLLMAPI sigue funcionando en segundo plano")
            .setSmallIcon(R.drawable.ic_notif)
            .setContentIntent(open)
            .setOngoing(true)
            .addAction(Notification.Action.Builder(null, "Salir", stop).build())
            .build()
    }

    // ---- TerminalSessionClient: se reenvía a la actividad si está visible ----

    override fun onTextChanged(changedSession: TerminalSession) { ui?.onTextChanged(changedSession) }
    override fun onTitleChanged(changedSession: TerminalSession) { ui?.onTitleChanged(changedSession) }
    override fun onSessionFinished(finishedSession: TerminalSession) {
        val code = finishedSession.exitStatus
        val text = runCatching { finishedSession.emulator?.screen?.transcriptText }.getOrNull().orEmpty()
        val crash = runCatching { File(filesDir, "crash.txt").readText() }.getOrNull().orEmpty()
        if (crash.isNotBlank()) Log.e(TAG, "Fallo de Infinito:\n" + crash.take(4000))
        Log.i(TAG, "Infinito ($mode) terminó (código $code). Pantalla:\n" + text.trim().take(3000) + "\n…\n" + text.takeLast(1500))
        if (finishedSession !== session) return
        switching?.let { next ->
            switching = null
            session = null
            next()
            return
        }
        if (mode == "gui") {
            guiListener?.invoke(code)
            if (code == 0 && guiListener == null) stopEverything()
            return
        }
        if (code == 0) {
            ui?.onSessionFinished(finishedSession)
            stopEverything()
            return
        }
        // Si falla, dejamos el mensaje a la vista y un toque lo vuelve a abrir.
        val msg = "\r\n\u001b[1;31m[Infinito se ha cerrado con un error (código $code). Toca la pantalla para reiniciarlo.]\u001b[0m\r\n"
        runCatching { finishedSession.emulator?.append(msg.toByteArray(), msg.toByteArray().size) }
        ui?.onTextChanged(finishedSession)
    }

    /** Arranca de nuevo Infinito tras un cierre con error. */
    fun restartSession(): TerminalSession {
        session = null
        return startSession(mode.ifEmpty { "tui" })
    }
    override fun onCopyTextToClipboard(session: TerminalSession, text: String?) {
        if (text.isNullOrEmpty()) return
        val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        cm.setPrimaryClip(ClipData.newPlainText("Infinito", text))
    }
    override fun onPasteTextFromClipboard(session: TerminalSession?) {
        val cm = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        val t = cm.primaryClip?.getItemAt(0)?.coerceToText(this)?.toString() ?: return
        session?.emulator?.paste(t)
    }
    override fun onBell(session: TerminalSession) {}
    override fun onColorsChanged(session: TerminalSession) { ui?.onColorsChanged(session) }
    override fun onTerminalCursorStateChange(state: Boolean) { ui?.onTerminalCursorStateChange(state) }
    override fun getTerminalCursorStyle(): Int? = null
    override fun logError(tag: String?, message: String?) { Log.e(tag ?: TAG, message ?: "") }
    override fun logWarn(tag: String?, message: String?) { Log.w(tag ?: TAG, message ?: "") }
    override fun logInfo(tag: String?, message: String?) { Log.i(tag ?: TAG, message ?: "") }
    override fun logDebug(tag: String?, message: String?) {}
    override fun logVerbose(tag: String?, message: String?) {}
    override fun logStackTraceWithMessage(tag: String?, message: String?, e: Exception?) { Log.e(tag ?: TAG, message, e) }
    override fun logStackTrace(tag: String?, e: Exception?) { Log.e(tag ?: TAG, "error", e) }

    companion object {
        const val TAG = "Infinito"
        const val CHANNEL = "infinito"
        const val NOTIF_ID = 1
        const val ACTION_STOP = "app.infinito.tui.STOP"
    }
}
