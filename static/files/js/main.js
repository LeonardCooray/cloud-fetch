import { render } from "preact";
import { html } from "./html.js";
import { App } from "./components/App.js";
import { startSync } from "./state.js";
import { createApi } from "./lib/api.js";
import { installSpaceToggle } from "./keys.js";

startSync();
installSpaceToggle();
const api = createApi(window.fetch.bind(window));
render(html`<${App} api=${api} />`, document.getElementById("app"));
