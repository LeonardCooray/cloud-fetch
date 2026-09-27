import { html } from "../html.js";
import { Icon } from "../icons.js";

export function ErrorBanner({ message, onDismiss }) {
  return html`<div class="banner" role="alert">
    <span>${message}</span>
    <button type="button" class="icon-btn" aria-label="Dismiss" onClick=${onDismiss}><${Icon} name="x" /></button>
  </div>`;
}
