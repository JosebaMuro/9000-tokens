package app.infinito.tui

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
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
 * Mantiene Infinito (y las pasarelas que arranque) vivo aunque la app pase a
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

    fun ensureSession(): TerminalSession {
        session?.let { return it }
        val files = filesDir.absolutePath
        val home = File(filesDir, "home").apply { mkdirs() }
        val tmp = cacheDir.apply { mkdirs() }
        val openFile = File(filesDir, "open-url")
        val bin = File(applicationInfo.nativeLibraryDir, "libinfinito.so").absolutePath
        val env = arrayOf(
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
            "INFINITO_ANDROID_APP=1",
            "INFINITO_OPEN_FILE=${openFile.absolutePath}",
        )
        watchOpenUrl(openFile)
        val s = TerminalSession(bin, home.absolutePath, arrayOf("infinito"), env, 3000, this)
        session = s
        acquireWakeLock()
        return s
    }

    /** Infinito escribe aquí una URL cuando pulsas "Panel web". */
    private fun watchOpenUrl(f: File) {
        urlObserver?.stopWatching()
        @Suppress("DEPRECATION")
        val obs = object : FileObserver(f.parent!!, CLOSE_WRITE) {
            override fun onEvent(event: Int, path: String?) {
                if (path != f.name) return
                val url = runCatching { f.readText().trim() }.getOrNull() ?: return
                f.delete()
                if (url.isEmpty()) return
                main.post {
                    runCatching {
                        startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                    }
                }
            }
        }
        obs.startWatching()
        urlObserver = obs
    }

    private fun acquireWakeLock() {
        if (wakeLock != null) return
        val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "infinito:pasarelas").apply {
            setReferenceCounted(false)
            acquire()
        }
    }

    fun stopEverything() {
        session?.finishIfRunning()
        session = null
        urlObserver?.stopWatching()
        wakeLock?.let { if (it.isHeld) it.release() }
        wakeLock = null
        stopForeground(true)
        stopSelf()
    }

    override fun onDestroy() {
        session?.finishIfRunning()
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
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val stop = PendingIntent.getService(
            this, 1, Intent(this, InfinitoService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val b = if (Build.VERSION.SDK_INT >= 26) Notification.Builder(this, CHANNEL) else
            @Suppress("DEPRECATION") Notification.Builder(this)
        return b.setContentTitle("Infinito en marcha")
            .setContentText("Las pasarelas siguen funcionando en segundo plano")
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
        Log.i(TAG, "Infinito terminó (código $code). Pantalla:\n" + text.trim().take(3000) + "\n…\n" + text.takeLast(1500))
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
        return ensureSession()
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
