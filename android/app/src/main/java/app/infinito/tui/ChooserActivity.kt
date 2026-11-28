package app.infinito.tui

import android.app.Activity
import android.content.res.ColorStateList
import android.content.res.Configuration
import android.graphics.Color
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Bundle
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.widget.CheckBox
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView

/** Pantalla inicial: elegir la app gráfica (GUI) o la de terminal (TUI). */
class ChooserActivity : Activity() {

    private val dark get() = (resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES
    private val bg get() = if (dark) 0xFF141218.toInt() else 0xFFFDF8FF.toInt()
    private val fg get() = if (dark) 0xFFE6E0EA.toInt() else 0xFF1C1B22.toInt()
    private val sub get() = if (dark) 0xFFCAC4D0.toInt() else 0xFF48454F.toInt()
    private val card get() = if (dark) 0xFF221F28.toInt() else 0xFFF0EAF7.toInt()
    private val accent get() = if (dark) 0xFFC9B8FF.toInt() else 0xFF5B3FD6.toInt()
    private val accentC get() = if (dark) 0xFF46309E.toInt() else 0xFFE7DEFF.toInt()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        var remembered = Modes.remembered(this)
        // Desde la notificación: volver a lo que estaba abierto.
        if (intent?.getBooleanExtra("resume", false) == true && Modes.last(this).isNotEmpty()) remembered = Modes.last(this)
        if (remembered.isNotEmpty() && intent?.getBooleanExtra("choose", false) != true) {
            Modes.open(this, remembered)
            finish()
            return
        }
        window.statusBarColor = bg
        window.navigationBarColor = bg
        if (!dark) window.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR

        val col = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER_HORIZONTAL
            setPadding(dp(24), dp(48), dp(24), dp(32))
        }
        col.addView(text("♾", 56f, accent).apply { gravity = Gravity.CENTER })
        col.addView(text("Infinito", 32f, fg).apply { gravity = Gravity.CENTER })
        col.addView(text("¿Cómo quieres usarlo?", 16f, sub).apply {
            gravity = Gravity.CENTER
            setPadding(0, dp(4), 0, dp(28))
        })
        val remember = CheckBox(this).apply {
            text = "Recordar mi elección"
            setTextColor(fg)
            textSize = 16f
            buttonTintList = ColorStateList.valueOf(accent)
            isChecked = false
        }
        fun pick(mode: String) {
            if (remember.isChecked) Modes.remember(this, mode) else Modes.remember(this, "")
            Modes.open(this, mode)
            finish()
        }
        col.addView(option("▣", "Aplicación", "Chat visual con imágenes, vídeo y voz, como una app moderna.", true) { pick("gui") })
        col.addView(option("❯_", "Terminal", "La interfaz de texto de siempre. Funciona con teclado y toques.", false) { pick("tui") })
        col.addView(remember, LinearLayout.LayoutParams(-2, -2).apply { topMargin = dp(16) })
        col.addView(text("Puedes cambiar de modo cuando quieras: la conversación es la misma.", 13f, sub).apply {
            gravity = Gravity.CENTER
            setPadding(0, dp(12), 0, 0)
        })
        val scroll = ScrollView(this).apply {
            setBackgroundColor(bg)
            isFillViewport = true
            addView(col)
        }
        setContentView(scroll)
    }

    private fun dp(v: Int) = TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v.toFloat(), resources.displayMetrics).toInt()

    private fun text(s: String, size: Float, color: Int) = TextView(this).apply {
        text = s
        textSize = size
        setTextColor(color)
    }

    private fun option(icon: String, title: String, desc: String, primary: Boolean, onClick: () -> Unit): View {
        val box = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(20), dp(20), dp(20), dp(20))
            background = GradientDrawable().apply {
                cornerRadius = dp(24).toFloat()
                setColor(if (primary) accentC else card)
            }
            isClickable = true
            isFocusable = true
            foreground = android.graphics.drawable.RippleDrawable(ColorStateList.valueOf(Color.argb(40, 128, 128, 128)), null, GradientDrawable().apply {
                cornerRadius = dp(24).toFloat(); setColor(Color.WHITE)
            })
            setOnClickListener { onClick() }
        }
        val ic = TextView(this).apply {
            text = icon
            textSize = 26f
            setTextColor(accent)
            typeface = Typeface.MONOSPACE
            gravity = Gravity.CENTER
            minWidth = dp(48)
        }
        val texts = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(16), 0, 0, 0)
            addView(text(title, 20f, fg).apply { typeface = Typeface.create("sans-serif-medium", Typeface.NORMAL) })
            addView(text(desc, 14f, sub))
        }
        box.addView(ic)
        box.addView(texts, LinearLayout.LayoutParams(0, -2, 1f))
        return box.apply {
            layoutParams = LinearLayout.LayoutParams(-1, -2).apply { bottomMargin = dp(12) }
        }
    }
}
