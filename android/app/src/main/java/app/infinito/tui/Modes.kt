package app.infinito.tui

import android.app.Activity
import android.content.Context
import android.content.Intent

/** Recuerda si se abre la interfaz gráfica o la de terminal. */
object Modes {
    private fun prefs(c: Context) = c.getSharedPreferences("modo", Context.MODE_PRIVATE)

    fun remembered(c: Context): String = prefs(c).getString("start", "") ?: ""

    fun remember(c: Context, mode: String) {
        prefs(c).edit().putString("start", mode).apply()
    }

    fun last(c: Context): String = prefs(c).getString("last", "") ?: ""

    /** Abre la pantalla del modo pedido. Si se recordaba otro modo, recuerda este. */
    fun open(a: Activity, mode: String) {
        if (remembered(a).isNotEmpty()) remember(a, mode)
        prefs(a).edit().putString("last", mode).apply()
        val cls = if (mode == "gui") GuiActivity::class.java else MainActivity::class.java
        a.startActivity(Intent(a, cls).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK))
    }
}
