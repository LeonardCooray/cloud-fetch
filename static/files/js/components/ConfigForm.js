import { useState } from "preact/hooks";
import { html } from "../html.js";
import { addSpaces, inputType } from "../lib/format.js";

// Edits a copy taken when the form opens, so the server's once-a-second
// pushes can't overwrite typing and Cancel discards everything.
export function ConfigForm({ config, api, onError, onClose }) {
  const [draft, setDraft] = useState(() => ({ ...config }));
  const [saving, setSaving] = useState(false);
  const [fieldError, setFieldError] = useState(null);
  const set = (k, v) => setDraft((d) => ({ ...d, [k]: v }));

  // number fields hold the raw text while editing (so clearing one doesn't
  // snap to 0); convert and check them only when saving
  const validated = () => {
    const out = {};
    for (const k of Object.keys(draft)) {
      if (inputType(config[k]) !== "number") {
        out[k] = draft[k];
        continue;
      }
      const text = String(draft[k]).trim();
      const n = Number(text);
      const whole = text !== "" && Number.isInteger(n);
      if (k === "IncomingPort" && (!whole || n < 1 || n > 65535)) return { error: "Enter a port between 1 and 65535" };
      if (!whole) return { error: `Enter a whole number for ${addSpaces(k)}` };
      out[k] = n;
    }
    return { config: out };
  };

  const save = async () => {
    const { config: out, error } = validated();
    setFieldError(error || null);
    if (error) return;
    setSaving(true);
    try {
      await api.configure(out);
      onClose();
    } catch (e) {
      onError(e);
    } finally {
      setSaving(false);
    }
  };

  return html`<form class="card config" onSubmit=${(e) => { e.preventDefault(); save(); }}>
    <h4>Configuration</h4>
    ${Object.keys(draft).map((k) => {
      const v = draft[k];
      const type = inputType(config[k]);
      if (type === "checkbox") {
        return html`<label class="check" key=${k}>
          <input type="checkbox" checked=${v} onChange=${(e) => set(k, e.currentTarget.checked)} /> ${addSpaces(k)}
        </label>`;
      }
      return html`<label key=${k}>${addSpaces(k)}
        <input type=${type} value=${v}
          onInput=${(e) => set(k, e.currentTarget.value)} />
      </label>`;
    })}
    ${fieldError && html`<p class="field-error" role="alert">${fieldError}</p>`}
    <div class="actions">
      <button type="submit" class="primary" disabled=${saving}>${saving ? "Saving…" : "Save"}</button>
      <button type="button" onClick=${onClose}>Cancel</button>
    </div>
  </form>`;
}
