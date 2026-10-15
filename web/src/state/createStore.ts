import { useSyncExternalStore } from "react";

/** ~25-line external store: get / set(partial|fn) / subscribe, plus a selector hook. */
export interface Store<T> {
  get: () => T;
  set: (patch: Partial<T> | ((s: T) => Partial<T>)) => void;
  subscribe: (cb: () => void) => () => void;
  reset: () => void;
}

export function createStore<T extends object>(initial: () => T): Store<T> {
  let state = initial();
  const subs = new Set<() => void>();
  return {
    get: () => state,
    set(patch) {
      const p = typeof patch === "function" ? patch(state) : patch;
      state = { ...state, ...p };
      subs.forEach((s) => s());
    },
    subscribe(cb) {
      subs.add(cb);
      return () => subs.delete(cb);
    },
    reset() {
      state = initial();
      subs.forEach((s) => s());
    },
  };
}

/** Selectors MUST return referentially-stable values (slice of state, not a freshly built object/array). */
export function useStore<T extends object, S>(store: Store<T>, selector: (s: T) => S): S {
  return useSyncExternalStore(store.subscribe, () => selector(store.get()));
}
