const FOCUS_HANDLES_SPACE = /^(INPUT|TEXTAREA|SELECT|BUTTON|AUDIO|VIDEO)$/;

// Space plays or pauses the first audio/video on screen, unless a control
// has focus: a focused button, field or player already acts on Space itself.
export function installSpaceToggle(doc = document, win = window) {
  doc.addEventListener("keydown", (e) => {
    if (e.key !== " " || e.repeat) return;
    const el = doc.activeElement;
    if (el && FOCUS_HANDLES_SPACE.test(el.tagName)) return;
    const height = win.innerHeight;
    for (const media of doc.querySelectorAll("video, audio")) {
      const r = media.getBoundingClientRect();
      const inView = (r.top >= 0 && r.top <= height) || (r.bottom >= 0 && r.bottom <= height);
      if (!inView) continue;
      if (media.paused) {
        // autoplay rules can refuse; the player's own button still works
        const p = media.play();
        if (p && p.catch) p.catch(() => {});
      } else media.pause();
      e.preventDefault();
      return;
    }
  });
}
