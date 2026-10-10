import { useEffect, useRef, useState } from "preact/hooks";
import { html } from "../html.js";
import { Icon } from "../icons.js";
import { copyText } from "../lib/clipboard.js";
import { DEFAULT_TTL, SHARE_TTLS, shareText } from "../lib/share.js";
import { CopyFallback } from "./CopyFallback.js";

// ShareButton fetches signed links for every expiry when its menu opens, so
// picking one copies straight away: browsers (Safari especially) refuse the
// clipboard once a click has waited on the network.
export function ShareButton({ getPaths, name, api, onError }) {
  const [state, setState] = useState("closed"); // closed | loading | open | copied | manual
  const [links, setLinks] = useState(null);
  const [text, setText] = useState("");
  const [chosen, setChosen] = useState("");
  const button = useRef(null);
  const menu = useRef(null);
  const timer = useRef(0);
  useEffect(() => () => clearTimeout(timer.current), []);

  const close = (restoreFocus) => {
    setState("closed");
    if (restoreFocus && button.current) button.current.focus();
  };

  useEffect(() => {
    if (state !== "open") return undefined;
    const first = menu.current && menu.current.querySelectorAll("button")[DEFAULT_TTL];
    if (first) first.focus();
    const onDown = (e) => {
      if (menu.current && !menu.current.contains(e.target) && !button.current.contains(e.target)) close(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [state]);

  const toggle = async () => {
    if (state === "open" || state === "loading") {
      close(false);
      return;
    }
    clearTimeout(timer.current);
    setState("loading");
    try {
      const res = await api.share(getPaths(), SHARE_TTLS.map((t) => t.seconds));
      setLinks(res.Links);
      setState("open");
    } catch (e) {
      setState("closed");
      onError(e);
    }
  };

  const choose = async (i) => {
    const value = shareText(links[i], window.location.href);
    setText(value);
    setChosen(SHARE_TTLS[i].label);
    if ((await copyText(value)) === "copied") {
      setState("copied");
      if (button.current) button.current.focus();
      timer.current = setTimeout(() => setState("closed"), 2000);
    } else {
      setState("manual");
    }
  };

  // Esc is handled here rather than on document so it goes no further
  const onKeyDown = (e) => {
    if (e.key !== "Escape") return;
    e.preventDefault();
    e.stopPropagation();
    close(true);
  };

  const copied = state === "copied";
  const expanded = state === "open" || state === "loading";
  return html`<span class="copy">
    <button ref=${button} type="button" class=${"icon-btn" + (copied ? " copied" : expanded ? " on" : "")}
      aria-label=${"Share " + name} aria-expanded=${expanded} onClick=${toggle}>
      <${Icon} name=${copied ? "check" : state === "loading" ? "loader" : "link"} class=${state === "loading" ? "spin" : ""} />${copied
        && html`<span class="copied-text" aria-hidden="true">${chosen}</span>`}
    </button>
    <span class="sr-only" aria-live="polite">${copied ? `Share link copied, lasts ${chosen}` : ""}</span>
    ${state === "open" && html`<div ref=${menu} class="share-menu" role="group" aria-label=${"Share " + name}
      onKeyDown=${onKeyDown}>
      <span class="muted">Anyone with the link can download it, no login needed, for</span>
      <div class="share-ttls">
        ${SHARE_TTLS.map((t, i) => html`<button type="button" key=${t.seconds}
          class=${i === DEFAULT_TTL ? "primary" : ""} onClick=${() => choose(i)}>${t.label}</button>`)}
      </div>
    </div>`}
    ${state === "manual" && html`<${CopyFallback} text=${text} title=${"Share link, lasts " + chosen}
      onClose=${(restoreFocus) => close(restoreFocus)} />`}
  </span>`;
}
