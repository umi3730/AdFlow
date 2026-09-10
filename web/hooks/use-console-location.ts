'use client';

import { useMemo, useSyncExternalStore } from 'react';
import { consoleURL, parseConsoleLocation } from '@/lib/console-location';

const changed = 'adflow:location';
function subscribe(listener: () => void) {
  window.addEventListener('popstate', listener);
  window.addEventListener(changed, listener);
  return () => {
    window.removeEventListener('popstate', listener);
    window.removeEventListener(changed, listener);
  };
}

export function updateConsoleLocation(
  patch: Record<string, string>,
  push = false,
) {
  const next = consoleURL(window.location.href, patch);
  if (
    next ===
    window.location.pathname + window.location.search + window.location.hash
  )
    return;
  window.history[push ? 'pushState' : 'replaceState'](
    window.history.state,
    '',
    next,
  );
  window.dispatchEvent(new Event(changed));
}

export function useConsoleLocation() {
  const search = useSyncExternalStore(
    subscribe,
    () => window.location.search,
    () => '',
  );
  return useMemo(() => parseConsoleLocation(search), [search]);
}
