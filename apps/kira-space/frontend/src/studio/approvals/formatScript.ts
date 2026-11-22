// Display-only script formatter: js-beautify, loaded on first use. The executed and hashed text is never
// the formatted one.
type Beautify = (src: string, opts: Record<string, unknown>) => string;

let beautify: Promise<Beautify> | undefined;

function loadBeautify(): Promise<Beautify> {
  if (!beautify) {
    beautify = import('js-beautify/js/lib/beautify.js')
      .then((m) => {
        const mod = m as { js_beautify?: Beautify; default?: { js_beautify: Beautify } };
        const fn = mod.js_beautify ?? mod.default?.js_beautify;
        if (!fn) throw new Error('js-beautify missing js_beautify');
        return fn;
      })
      .catch((err) => {
        beautify = undefined;
        throw err;
      });
  }
  return beautify;
}

const OPTIONS = {
  indent_size: 2,
  wrap_line_length: 0,
  break_chained_methods: true,
  end_with_newline: false,
  preserve_newlines: true,
  max_preserve_newlines: 2,
};

/** The script as shown. `formatted` is true only when the text differs from the source. A failed import
 *  shows the raw text. */
export async function formatScript(src: string): Promise<{ text: string; formatted: boolean }> {
  try {
    const text = (await loadBeautify())(src, OPTIONS);
    return { text, formatted: text !== src };
  } catch {
    return { text: src, formatted: false };
  }
}
