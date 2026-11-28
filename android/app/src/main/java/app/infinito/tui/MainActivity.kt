package app.infinito.tui

import android.app.Activity
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.graphics.Color
import android.graphics.Typeface
import android.os.Build
import android.os.Bundle
import android.os.IBinder
import android.util.TypedValue
import android.view.Gravity
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.View
import android.view.ViewGroup
import android.view.inputmethod.InputMethodManager
import android.widget.Button
import android.widget.HorizontalScrollView
import android.widget.LinearLayout
import com.termux.terminal.TerminalSession
import com.termux.terminal.TerminalSessionClient
import com.termux.view.TerminalView
import com.termux.view.TerminalViewClient

class MainActivity : Activity(), ServiceConnection, TerminalViewClient, TerminalSessionClient {

    private lateinit var terminal: TerminalView
    private var service: InfinitoService? = null
    private var ctrlOn = false
    private var altOn = false
    private var ctrlBtn: Button? = null
    private var altBtn: Button? = null
    private var fontSize = 0

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val prefs = getPreferences(Context.MODE_PRIVATE)
        fontSize = prefs.getInt("font", dp(13f).toInt())

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(getColor(R.color.bg))
        }
        terminal = TerminalView(this, null).apply {
            setTerminalViewClient(this@MainActivity)
            setTextSize(fontSize)
            setTypeface(Typeface.MONOSPACE)
            isFocusable = true
            isFocusableInTouchMode = true
            keepScreenOn = true
        }
        root.addView(terminal, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f))
        root.addView(extraKeys())
        setContentView(root)

        val i = Intent(this, InfinitoService::class.java)
        if (Build.VERSION.SDK_INT >= 26) startForegroundService(i) else startService(i)
        bindService(i, this, Context.BIND_AUTO_CREATE)
    }

    override fun onDestroy() {
        service?.ui = null
        runCatching { unbindService(this) }
        super.onDestroy()
    }

    override fun onServiceConnected(name: ComponentName?, binder: IBinder?) {
        val s = (binder as InfinitoService.LocalBinder).service
        service = s
        s.ui = this
        s.requestSession("tui") { session ->
            terminal.attachSession(session)
            terminal.requestFocus()
            terminal.onScreenUpdated()
        }
    }

    override fun onServiceDisconnected(name: ComponentName?) {
        service = null
    }

    // ---------- Fila de teclas extra (Esc, Tab, Ctrl, flechas…) ----------

    private fun dp(v: Float) = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v, resources.displayMetrics)

    private fun key(label: String, action: (Button) -> Unit): Button = Button(this).apply {
        text = label
        isAllCaps = false
        setTextColor(getColor(R.color.fg))
        setBackgroundColor(getColor(R.color.key))
        textSize = 15f
        minWidth = dp(44f).toInt()
        minimumWidth = dp(44f).toInt()
        setPadding(dp(8f).toInt(), 0, dp(8f).toInt(), 0)
        isFocusable = false
        setOnClickListener { action(this) }
        layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, dp(44f).toInt()).apply {
            setMargins(dp(2f).toInt(), dp(2f).toInt(), dp(2f).toInt(), dp(2f).toInt())
        }
    }

    private fun send(s: String) {
        service?.session?.write(s.toByteArray(), 0, s.toByteArray().size)
    }

    private fun arrow(code: Int) {
        terminal.handleKeyCode(code, 0)
    }

    private fun extraKeys(): View {
        val row = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setBackgroundColor(getColor(R.color.bar))
        }
        row.addView(key("▣ GUI") { Modes.open(this, "gui"); finish() })
        row.addView(key("⌨") { toggleKeyboard() })
        row.addView(key("Esc") { send("\u001b") })
        row.addView(key("Tab") { send("\t") })
        ctrlBtn = key("Ctrl") { ctrlOn = !ctrlOn; paintMods() }
        row.addView(ctrlBtn)
        altBtn = key("Alt") { altOn = !altOn; paintMods() }
        row.addView(altBtn)
        row.addView(key("←") { arrow(KeyEvent.KEYCODE_DPAD_LEFT) })
        row.addView(key("↑") { arrow(KeyEvent.KEYCODE_DPAD_UP) })
        row.addView(key("↓") { arrow(KeyEvent.KEYCODE_DPAD_DOWN) })
        row.addView(key("→") { arrow(KeyEvent.KEYCODE_DPAD_RIGHT) })
        row.addView(key("Pág↑") { arrow(KeyEvent.KEYCODE_PAGE_UP) })
        row.addView(key("Pág↓") { arrow(KeyEvent.KEYCODE_PAGE_DOWN) })
        row.addView(key("A+") { zoom(1) })
        row.addView(key("A−") { zoom(-1) })
        row.addView(key("Pegar") { service?.let { s -> s.onPasteTextFromClipboard(s.session) } })
        return HorizontalScrollView(this).apply {
            isHorizontalScrollBarEnabled = false
            setBackgroundColor(getColor(R.color.bar))
            addView(row)
        }
    }

    private fun paintMods() {
        ctrlBtn?.setBackgroundColor(getColor(if (ctrlOn) R.color.keyOn else R.color.key))
        altBtn?.setBackgroundColor(getColor(if (altOn) R.color.keyOn else R.color.key))
    }

    private fun toggleKeyboard() {
        val imm = getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager
        terminal.requestFocus()
        @Suppress("DEPRECATION")
        imm.toggleSoftInput(InputMethodManager.SHOW_FORCED, 0)
    }

    private fun zoom(d: Int) {
        val step = dp(1f).toInt().coerceAtLeast(1)
        fontSize = (fontSize + d * step).coerceIn(dp(7f).toInt(), dp(32f).toInt())
        terminal.setTextSize(fontSize)
        getPreferences(Context.MODE_PRIVATE).edit().putInt("font", fontSize).apply()
    }

    // ---------- TerminalViewClient ----------

    private var scaleAcc = 1f
    override fun onScale(scale: Float): Float {
        scaleAcc *= scale
        if (scaleAcc > 1.15f) { zoom(1); scaleAcc = 1f } else if (scaleAcc < 0.87f) { zoom(-1); scaleAcc = 1f }
        return 1f
    }

    override fun onSingleTapUp(e: MotionEvent?) {
        val cur = service?.session
        if (cur != null && !cur.isRunning) {
            service?.let { terminal.attachSession(it.restartSession()); terminal.onScreenUpdated() }
            return
        }
        // Con el ratón activo, el toque ya llega a Infinito como clic.
        val em = service?.session?.emulator
        if (em == null || !em.isMouseTrackingActive) toggleKeyboard()
    }

    override fun shouldBackButtonBeMappedToEscape(): Boolean = true
    override fun shouldEnforceCharBasedInput(): Boolean = true
    override fun shouldUseCtrlSpaceWorkaround(): Boolean = false
    override fun isTerminalViewSelected(): Boolean = true
    override fun copyModeChanged(copyMode: Boolean) {}
    override fun onKeyDown(keyCode: Int, e: KeyEvent?, session: TerminalSession?): Boolean = false
    override fun onKeyUp(keyCode: Int, e: KeyEvent?): Boolean = false
    override fun onLongPress(event: MotionEvent?): Boolean = false
    override fun readControlKey(): Boolean = ctrlOn
    override fun readAltKey(): Boolean = altOn
    override fun readShiftKey(): Boolean = false
    override fun readFnKey(): Boolean = false

    override fun onCodePoint(codePoint: Int, ctrlDown: Boolean, session: TerminalSession?): Boolean {
        // Ctrl y Alt de la barra valen para una sola tecla.
        if (ctrlOn || altOn) {
            terminal.post { ctrlOn = false; altOn = false; paintMods() }
        }
        return false
    }

    override fun onEmulatorSet() {
        terminal.setTerminalCursorBlinkerRate(0)
    }

    // ---------- TerminalSessionClient (reenviado por el servicio) ----------

    override fun onTextChanged(changedSession: TerminalSession) { terminal.onScreenUpdated() }
    override fun onTitleChanged(changedSession: TerminalSession) {}
    override fun onSessionFinished(finishedSession: TerminalSession) { finishAndRemoveTask() }
    override fun onCopyTextToClipboard(session: TerminalSession, text: String?) {}
    override fun onPasteTextFromClipboard(session: TerminalSession?) {}
    override fun onBell(session: TerminalSession) {}
    override fun onColorsChanged(session: TerminalSession) { terminal.invalidate() }
    override fun onTerminalCursorStateChange(state: Boolean) {}
    override fun getTerminalCursorStyle(): Int? = null

    override fun logError(tag: String?, message: String?) {}
    override fun logWarn(tag: String?, message: String?) {}
    override fun logInfo(tag: String?, message: String?) {}
    override fun logDebug(tag: String?, message: String?) {}
    override fun logVerbose(tag: String?, message: String?) {}
    override fun logStackTraceWithMessage(tag: String?, message: String?, e: Exception?) {}
    override fun logStackTrace(tag: String?, e: Exception?) {}
}
