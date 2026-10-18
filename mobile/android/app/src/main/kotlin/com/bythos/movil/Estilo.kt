package com.bythos.movil

import android.app.Activity
import android.content.Context
import android.graphics.drawable.GradientDrawable
import android.text.InputType
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat

/** Color semántico del texto: tinta normal, verde de éxito o rojo de error. */
enum class Tono { NORMAL, EXITO, ERROR }

fun Context.dp(valor: Int): Int = (valor * resources.displayMetrics.density).toInt()

/**
 * Fábrica de vistas con el aspecto del escritorio. Todo color, tamaño y radio sale de los
 * recursos (colors/dimens/themes); aquí solo se arman las vistas, que siguen siendo Views reales.
 */
object Estilo {

    private fun texto(c: Context, t: CharSequence, apariencia: Int) =
        TextView(c).apply { text = t; setTextAppearance(apariencia) }

    fun titulo(c: Context, t: CharSequence) = texto(c, t, R.style.TextAppearance_Bythos_Titulo)
    fun panel(c: Context, t: CharSequence) = texto(c, t, R.style.TextAppearance_Bythos_Panel)
    fun cuerpo(c: Context, t: CharSequence = "") = texto(c, t, R.style.TextAppearance_Bythos_Cuerpo)
    fun tenue(c: Context, t: CharSequence) = texto(c, t, R.style.TextAppearance_Bythos_Tenue)

    private fun boton(c: Context, t: CharSequence, estilo: Int, alPulsar: () -> Unit) =
        Button(c, null, 0, estilo).apply { text = t; setOnClickListener { alPulsar() } }

    fun botonPrimario(c: Context, t: CharSequence, alPulsar: () -> Unit): Button = boton(c, t, R.style.Boton_Primario, alPulsar)
    fun botonSecundario(c: Context, t: CharSequence, alPulsar: () -> Unit): Button = boton(c, t, R.style.Boton_Secundario, alPulsar)

    fun campo(c: Context): EditText = EditText(c).apply {
        setTextAppearance(R.style.TextAppearance_Bythos_Cuerpo)
        setBackgroundResource(R.drawable.campo)
        setPadding(c.dp(12), c.dp(12), c.dp(12), c.dp(12))
        minHeight = c.resources.getDimensionPixelSize(R.dimen.objetivo_tactil)
        inputType = InputType.TYPE_CLASS_TEXT
    }

    fun tarjeta(c: Context, vararg hijos: View): LinearLayout = columna(c).apply {
        setBackgroundResource(R.drawable.tarjeta)
        val p = c.resources.getDimensionPixelSize(R.dimen.tarjeta_padding)
        setPadding(p, p, p, p)
        hijos.forEach { addView(it) }
    }

    /** Franja `bg_alt` con la marca; [pantalla] le suma los insets del sistema. */
    fun encabezado(c: Context): View = LinearLayout(c).apply {
        setBackgroundResource(R.color.bg_alt)
        addView(TextView(c).apply {
            text = "BYTHOS"
            textSize = 14f
            setTypeface(typeface, android.graphics.Typeface.BOLD)
            letterSpacing = 0.24f
            setTextColor(c.getColor(R.color.ink_accent))
        })
    }

    fun tono(v: TextView, t: Tono) = v.setTextColor(
        v.context.getColor(
            when (t) {
                Tono.NORMAL -> R.color.ink
                Tono.EXITO -> R.color.green
                Tono.ERROR -> R.color.red
            },
        ),
    )

    /** Columna vertical con separación uniforme entre hijos visibles. */
    fun columna(c: Context): LinearLayout = LinearLayout(c).apply {
        orientation = LinearLayout.VERTICAL
        showDividers = LinearLayout.SHOW_DIVIDER_MIDDLE
        dividerDrawable = GradientDrawable().apply { setSize(0, c.resources.getDimensionPixelSize(R.dimen.separacion)) }
    }

    /**
     * Raíz de una pantalla: encabezado opcional + ScrollView con padding de 24dp. Aplica los insets
     * (barras del sistema, recorte, teclado) una sola vez para que nada quede bajo una barra.
     * Devuelve la columna de contenido, ya instalada con setContentView.
     */
    fun pantalla(a: Activity, conEncabezado: Boolean = true): LinearLayout {
        val pad = a.resources.getDimensionPixelSize(R.dimen.pantalla_padding)
        val cabecera = if (conEncabezado) encabezado(a) else null
        val contenido = columna(a).apply { setPadding(pad, pad, pad, pad) }
        val scroll = ScrollView(a).apply {
            isFillViewport = true
            addView(contenido)
        }
        val raiz = LinearLayout(a).apply {
            orientation = LinearLayout.VERTICAL
            cabecera?.let { addView(it) }
            addView(scroll, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f))
        }
        WindowCompat.setDecorFitsSystemWindows(a.window, false)
        ViewCompat.setOnApplyWindowInsetsListener(raiz) { _, wi ->
            val b = wi.getInsets(
                WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout() or WindowInsetsCompat.Type.ime(),
            )
            cabecera?.setPadding(pad + b.left, pad / 2 + b.top, pad + b.right, pad / 2)
            scroll.setPadding(b.left, if (cabecera == null) b.top else 0, b.right, b.bottom)
            WindowInsetsCompat.CONSUMED
        }
        a.setContentView(raiz)
        return contenido
    }
}
