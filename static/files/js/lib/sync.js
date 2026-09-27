// Holds the object velox merges server pushes into, plus a version counter.
// A subscriber passes the version it last rendered; if updates landed before
// it subscribed (velox can connect before the first effect runs), it is
// notified at once instead of waiting for the next change on the server.
export function createSyncStore() {
  const state = {};
  let version = 0;
  let connected = false;
  let everConnected = false;
  const subscribers = new Set();
  const emit = () => {
    version++;
    for (const fn of subscribers) fn(version);
  };
  return {
    state,
    get version() {
      return version;
    },
    snapshot: () => ({ connected, everConnected }),
    attach(v) {
      v.onupdate = emit;
      v.onchange = (c) => {
        connected = c;
        if (c) everConnected = true;
        emit();
      };
      return v;
    },
    subscribe(fn, seenVersion) {
      subscribers.add(fn);
      if (version !== seenVersion) fn(version);
      return () => subscribers.delete(fn);
    },
  };
}
