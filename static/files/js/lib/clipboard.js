// copyText never rejects: it resolves "copied", or "manual" when the caller
// should show the text for copying by hand. navigator.clipboard only exists
// in secure contexts and Cloud Fetch often runs on plain HTTP, so the old
// execCommand route is the fallback.
export async function copyText(text, env = browserEnv()) {
  if (env.secure && env.clipboard && env.clipboard.writeText) {
    try {
      await env.clipboard.writeText(text);
      return "copied";
    } catch {
      // permission refused or document not focused; try the old way
    }
  }
  return legacyCopy(text, env.document) ? "copied" : "manual";
}

function legacyCopy(text, doc) {
  if (!doc || !doc.body) return false;
  const previous = doc.activeElement;
  const area = doc.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.top = "0";
  area.style.left = "-9999px";
  area.style.opacity = "0";
  area.style.fontSize = "12pt"; // smaller text makes iOS zoom on focus
  doc.body.appendChild(area);
  try {
    area.focus();
    area.select();
    area.setSelectionRange(0, text.length); // iOS ignores select() on a readonly field
    return doc.execCommand("copy") === true;
  } catch {
    return false;
  } finally {
    doc.body.removeChild(area);
    if (previous && previous.focus) previous.focus();
  }
}

export function browserEnv() {
  return { secure: window.isSecureContext, clipboard: navigator.clipboard, document };
}
