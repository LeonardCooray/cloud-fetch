// Runs before first paint (a classic script, not a module) so a forced theme
// never flashes the system one. Mirrors readTheme/applyTheme in lib/theme.js.
(function () {
  try {
    var v = localStorage.getItem("tcTheme");
    if (v === "light" || v === "dark") document.documentElement.setAttribute("data-theme", v);
  } catch (e) {
    // storage unavailable: follow the system
  }
})();
