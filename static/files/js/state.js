import { useEffect, useState } from "preact/hooks";
import { createSyncStore } from "./lib/sync.js";

const store = createSyncStore();

export function startSync(velox = window.velox) {
  return store.attach(velox("/sync", store.state));
}

export function useSync() {
  const [seen, setSeen] = useState(store.version);
  useEffect(() => store.subscribe(setSeen, seen), []);
  return { state: store.state, ...store.snapshot() };
}
