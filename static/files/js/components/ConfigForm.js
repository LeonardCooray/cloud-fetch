import { useEffect, useRef, useState } from "preact/hooks";
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
    <${RevokeShares} api=${api} onError=${onError} />
  </form>`;
}

// Revoking replaces the signing key, so it takes a second, deliberate click.
function RevokeShares({ api, onError }) {
  const [state, setState] = useState("idle"); // idle | armed | revoking | done
  const armedAt = useRef(0);
  useEffect(() => {
    if (state !== "armed" && state !== "done") return undefined;
    const id = setTimeout(() => setState("idle"), state === "armed" ? 3000 : 4000);
    return () => clearTimeout(id);
  }, [state]);
  const click = async () => {
    if (state === "idle" || state === "done") {
      armedAt.current = Date.now();
      setState("armed");
      return;
    }
    if (state !== "armed" || Date.now() - armedAt.current < 400) return;
    setState("revoking");
    try {
      await api.revokeShares();
      setState("done");
    } catch (e) {
      setState("idle");
      onError(e);
    }
  };
  return html`<div class="share-revoke">
    <h4>Share links</h4>
    <p class="muted">Share links work without a login until they expire. Revoking stops every link shared so far.</p>
    <button type="button" class="danger" disabled=${state === "revoking"} onClick=${click}>
      ${state === "armed" ? "Click again to revoke all" : state === "revoking" ? "Revoking…" : "Revoke all share links"}
    </button>
    <span class="sr-only" aria-live="polite">${state === "done" ? "All share links revoked" : ""}</span>
    ${state === "done" && html`<span class="muted" aria-hidden="true"> All share links revoked</span>`}
  </div>`;
}
