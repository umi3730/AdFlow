const pauseKey = 'adflow.demo-auto-login-paused';

export function allowDemoAutoLogin(
  enabled: boolean,
  pageURL: string,
  apiURL: string,
  paused = false,
) {
  if (!enabled || paused) return false;
  const local = (value: string) => {
    try {
      const url = new URL(value);
      return (
        ['http:', 'https:'].includes(url.protocol) &&
        ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname) &&
        !url.username &&
        !url.password
      );
    } catch {
      return false;
    }
  };
  return local(pageURL) && local(apiURL);
}

// Only this user preference is persisted; never credentials or access tokens.
export function demoLoginPaused() {
  try {
    return sessionStorage.getItem(pauseKey) === 'true';
  } catch {
    return false;
  }
}
export function pauseDemoLogin() {
  try {
    sessionStorage.setItem(pauseKey, 'true');
  } catch {
    /* This page still remains signed out. */
  }
}
export function resumeDemoLogin() {
  try {
    sessionStorage.removeItem(pauseKey);
  } catch {
    /* Preference storage is optional. */
  }
}
