import { useSyncExternalStore } from "react";

// Routing for a handful of flat paths: the path lives in the address bar,
// navigate() pushes a history entry, and usePath() re-renders on every change,
// Back and Forward included.

function subscribe(onChange: () => void) {
  addEventListener("popstate", onChange);
  return () => removeEventListener("popstate", onChange);
}

export function usePath(): string {
  return useSyncExternalStore(subscribe, () => location.pathname);
}

export function navigate(to: string, { replace = false } = {}) {
  if (replace) {
    history.replaceState(null, "", to);
  } else {
    history.pushState(null, "", to);
    scrollTo(0, 0);
  }
  // pushState and replaceState fire no event, so announce the change the way
  // the Back button does.
  dispatchEvent(new PopStateEvent("popstate"));
}
