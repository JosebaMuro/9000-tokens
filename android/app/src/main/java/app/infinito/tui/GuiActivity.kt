package app.infinito.tui

import android.annotation.SuppressLint
import android.app.Activity
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.content.res.Configuration
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.webkit.JavascriptInterface
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView
import java.io.File
import java.net.HttpURLConnection
import java.net.URL

/** La interfaz gráfica de Infinito a pantalla completa (servida por el propio binario). */
class GuiActivity : Activity(), ServiceConnection {

    private lateinit var web: WebView
    private lateinit var overlay: LinearLayout
    private lateinit var status: TextView
    private lateinit var retry: Button
    private lateinit var spinner: ProgressBar
    private var service: InfinitoService? = null
    private val main = Handler(Looper.getMainLooper())
    @Volatile private var waiting = false
    private var loaded = false

    private val dark get() = (resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES
    private val bg get() = if (dark) 0xFF141218.toInt() else 0xFFFDF8FF.toInt()

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.statusBarColor = bg
        window.navigationBarColor = if (dark) 0xFF221F28.toInt() else 0xFFF0EAF7.toInt()
        if (!dark) window.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR or
            (if (Build.VERSION.SDK_INT >= 26) View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR else 0)

        web = WebView(this).apply {
            setBackgroundColor(bg)
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.mediaPlaybackRequiresUserGesture = false
            settings.cacheMode = WebSettings.LOAD_NO_CACHE
            settings.allowFileAccess = false
            if (Build.VERSION.SDK_INT >= 29) {
                @Suppress("DEPRECATION")
                settings.forceDark = WebSettings.FORCE_DARK_OFF
            }
            addJavascriptInterface(Bridge(), "Android")
            webViewClient = object : WebViewClient() {
                override fun shouldOverrideUrlLoading(view: WebView, req: WebResourceRequest): Boolean {
                    val u = req.url
                    if (u.host == "127.0.0.1" || u.host == "localhost") return false
                    runCatching { startActivity(Intent(Intent.ACTION_VIEW, u)) }
                    return true
                }
                override fun onPageFinished(view: WebView, url: String?) {
                    if (loaded) overlay.visibility = View.GONE
                }
            }
        }
        overlay = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            setBackgroundColor(bg)
            val pad = dp(32)
            setPadding(pad, pad, pad, pad)
        }
        val logo = TextView(this).apply {
            text = "♾"; textSize = 56f; gravity = Gravity.CENTER
            setTextColor(if (dark) 0xFFC9B8FF.toInt() else 0xFF5B3FD6.toInt())
        }
        spinner = ProgressBar(this)
        status = TextView(this).apply {
            text = "Abriendo Infinito…"; textSize = 16f; gravity = Gravity.CENTER
            setTextColor(if (dark) 0xFFCAC4D0.toInt() else 0xFF48454F.toInt())
            setPadding(0, dp(16), 0, dp(16))
        }
        retry = Button(this).apply {
            text = "Reintentar"; visibility = View.GONE
            setOnClickListener { restart() }
        }
        val toTui = Button(this).apply {
            text = "Ver la terminal"; visibility = View.GONE
            setOnClickListener { Modes.open(this@GuiActivity, "tui"); finish() }
        }
        overlay.addView(logo); overlay.addView(spinner); overlay.addView(status); overlay.addView(retry); overlay.addView(toTui)
        retry.tag = toTui

        val root = FrameLayout(this)
        root.addView(web, FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        root.addView(overlay, FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT))
        setContentView(root)

        val i = Intent(this, InfinitoService::class.java)
        if (Build.VERSION.SDK_INT >= 26) startForegroundService(i) else startService(i)
        bindService(i, this, Context.BIND_AUTO_CREATE)
    }

    private fun dp(v: Int) = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v.toFloat(), resources.displayMetrics).toInt()

    override fun onServiceConnected(name: ComponentName?, binder: IBinder?) {
        val s = (binder as InfinitoService.LocalBinder).service
        service = s
        s.guiListener = { code -> main.post { onExit(code) } }
        s.requestSession("gui") { waitAndLoad(s) }
    }

    override fun onServiceDisconnected(name: ComponentName?) { service = null }

    override fun onDestroy() {
        service?.guiListener = null
        waiting = false
        runCatching { unbindService(this) }
        web.destroy()
        super.onDestroy()
    }

    /** Espera a que el servidor local responda y carga la interfaz. */
    private fun waitAndLoad(s: InfinitoService) {
        val port = s.guiPort()
        val token = s.guiToken
        waiting = true
        Thread {
            val t0 = System.currentTimeMillis()
            while (waiting && System.currentTimeMillis() - t0 < 90_000) {
                val ok = runCatching {
                    val c = URL("http://127.0.0.1:$port/app/").openConnection() as HttpURLConnection
                    c.connectTimeout = 800; c.readTimeout = 1500
                    val code = c.responseCode
                    c.disconnect()
                    code == 200
                }.getOrDefault(false)
                if (ok) {
                    waiting = false
                    val theme = if (dark) "dark" else "light"
                    main.post {
                        loaded = true
                        web.loadUrl("http://127.0.0.1:$port/app/#t=$token&theme=$theme")
                    }
                    return@Thread
                }
                Thread.sleep(400)
            }
            if (waiting) main.post { showError("Infinito tarda demasiado en abrir.") }
        }.start()
    }

    private fun onExit(code: Int) {
        if (isFinishing) return
        if (code == 0) { finishAndRemoveTask(); return }
        val crash = runCatching { File(filesDir, "crash.txt").readText().lineSequence().firstOrNull { it.isNotBlank() } }.getOrNull()
        showError("Infinito se ha cerrado con un error (código $code)." + (crash?.let { "\n$it" } ?: ""))
    }

    private fun showError(msg: String) {
        waiting = false
        loaded = false
        overlay.visibility = View.VISIBLE
        spinner.visibility = View.GONE
        status.text = msg
        retry.visibility = View.VISIBLE
        (retry.tag as View).visibility = View.VISIBLE
    }

    private fun restart() {
        val s = service ?: return
        spinner.visibility = View.VISIBLE
        retry.visibility = View.GONE
        (retry.tag as View).visibility = View.GONE
        status.text = "Abriendo Infinito…"
        s.restartSession()
        waitAndLoad(s)
    }

    @Deprecated("Deprecated in Java")
    override fun onBackPressed() {
        // La GUI es una sola página: Atrás deja la app en segundo plano.
        moveTaskToBack(true)
    }

    /** Funciones que la página puede llamar como window.Android.*. */
    inner class Bridge {
        @JavascriptInterface
        fun switchMode(mode: String) {
            main.post { Modes.open(this@GuiActivity, if (mode == "tui") "tui" else "gui"); finish() }
        }

        @JavascriptInterface
        fun setStartMode(mode: String) {
            main.post { Modes.remember(this@GuiActivity, if (mode == "gui" || mode == "tui") mode else "") }
        }

        @JavascriptInterface
        fun saveFile(name: String) {
            val s = service ?: return
            if (name.isEmpty() || name.contains('/') || name.startsWith(".")) return
            val f = File(s.filesFolder(), name)
            if (f.isFile) Thread { s.saveToDownloads(f) }.start()
        }
    }
}
