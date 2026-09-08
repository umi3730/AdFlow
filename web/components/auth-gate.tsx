'use client';

import { createContext, useContext, useEffect, useRef, useState } from 'react';
import { LoaderCircle, ShieldCheck, LogOut } from 'lucide-react';
import { api, ApiError } from '@/lib/api';
import { pauseDemoLogin } from '@/lib/demo-login';
import { demoLoginDefaults, registrationError } from '@/lib/auth-form';
import {
  clearSession,
  roleAllows,
  roleLabels,
  sessionExpiry,
  sessionToken,
  setSession,
  subscribeSession,
  type AuthInfo,
  type Role,
} from '@/lib/auth-session';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';

const AuthContext = createContext<AuthInfo | null>(null);
export function useAccess() {
  const auth = useContext(AuthContext);
  return {
    auth,
    canOperate: roleAllows(auth?.principal.role, 'operator'),
    canAdmin: roleAllows(auth?.principal.role, 'admin'),
  };
}
export function RoleGate({
  minimum,
  children,
}: {
  minimum: Role;
  children: React.ReactNode;
}) {
  const { auth } = useAccess();
  return roleAllows(auth?.principal.role, minimum) ? (
    children
  ) : (
    <p className="rounded-lg border bg-muted p-4 text-sm text-muted-foreground">
      当前账号为{auth ? roleLabels[auth.principal.role] : '未登录'}，此功能需要
      {roleLabels[minimum]}权限。
    </p>
  );
}
export function SessionIdentity() {
  const { auth } = useAccess();
  if (!auth) return null;
  return (
    <div className="flex items-center gap-2 text-sm">
      <span className="max-w-32 truncate" title={auth.principal.username}>
        {auth.principal.username}
      </span>
      <span className="rounded-full bg-primary/10 px-2 py-1 text-xs text-primary">
        {roleLabels[auth.principal.role]}
      </span>
      {auth.authEnabled && (
        <Button
          type="button"
          size="icon"
          variant="ghost"
          aria-label="退出登录"
          title="退出登录"
          onClick={() => {
            pauseDemoLogin();
            clearSession();
          }}
        >
          <LogOut className="size-4" />
        </Button>
      )}
    </div>
  );
}

export function AuthGate({ children }: { children: React.ReactNode }) {
  const [auth, setAuth] = useState<AuthInfo | null>(null);
  const [checking, setChecking] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [unavailable, setUnavailable] = useState(false);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [canRegister, setCanRegister] = useState(false);
  const defaults = useRef(demoLoginDefaults(false));
  const generation = useRef(0);
  const submitting = useRef(false);

  useEffect(() => {
    let alive = true;
    const lifecycle = generation;
    const unsubscribe = subscribeSession(() => {
      if (!sessionExpiry() && alive) {
        generation.current++;
        setAuth(null);
        setMode('login');
        setUsername(defaults.current.username);
        setPassword(defaults.current.password);
        setConfirmPassword('');
      }
    });
    const attempt = ++generation.current;
    void api
      .authOptions()
      .then((options) => {
        if (!alive || attempt !== generation.current) return null;
        defaults.current = demoLoginDefaults(options.demoLoginPrefill);
        setUsername(defaults.current.username);
        setPassword(defaults.current.password);
        setCanRegister(options.registrationEnabled);
        return api.authMe();
      })
      .then((info) => {
        if (alive && attempt === generation.current) setAuth(info);
      })
      .catch((cause) => {
        if (!alive) return;
        if (!(cause instanceof ApiError && cause.status === 401)) {
          setUnavailable(true);
          setError('无法连接后端，请确认服务已启动后重试');
        }
      })
      .finally(() => {
        if (alive) setChecking(false);
      });
    return () => {
      alive = false;
      lifecycle.current++;
      unsubscribe();
    };
  }, []);

  useEffect(() => {
    if (!auth?.authEnabled) return;
    const expire = () => {
      if (!sessionToken()) setAuth(null);
    };
    const timer = setTimeout(expire, Math.max(0, sessionExpiry() - Date.now()));
    window.addEventListener('focus', expire);
    document.addEventListener('visibilitychange', expire);
    return () => {
      clearTimeout(timer);
      window.removeEventListener('focus', expire);
      document.removeEventListener('visibilitychange', expire);
    };
  }, [auth]);

  async function login(event: { preventDefault(): void }) {
    event.preventDefault();
    if (submitting.current) return;
    if (mode === 'register') {
      const validation = registrationError(username, password, confirmPassword);
      if (validation) {
        setError(validation);
        return;
      }
    }
    submitting.current = true;
    setBusy(true);
    setError('');
    const attempt = ++generation.current;
    try {
      const issued =
        mode === 'register'
          ? await api.register(username.trim(), password)
          : await api.login(username.trim(), password);
      if (attempt !== generation.current) return;
      setSession(issued);
      const info = await api.authMe();
      if (attempt !== generation.current) return;
      if (!info.authEnabled) {
        clearSession();
      }
      setPassword('');
      setConfirmPassword('');
      setAuth(info);
    } catch (cause) {
      if (sessionToken()) clearSession();
      setError(
        cause instanceof ApiError && cause.status === 401
          ? '用户名或密码不正确'
          : cause instanceof ApiError
            ? cause.message
            : '请求失败，请检查服务连接后重试',
      );
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }

  if (auth)
    return <AuthContext.Provider value={auth}>{children}</AuthContext.Provider>;
  return (
    <main className="console-login grid min-h-screen place-items-center px-4 py-8 sm:px-8">
      <div className="console-login-panel w-full max-w-sm overflow-hidden rounded-xl border bg-card">
        <section className="flex flex-col justify-center gap-6 p-7 sm:p-9">
          <div className="space-y-2">
            <ShieldCheck className="size-8 text-primary" />
            <h1 className="text-2xl font-semibold">
              {mode === 'login' ? '登录 AdFlow' : '注册 AdFlow'}
            </h1>
            <p className="text-sm text-muted-foreground">
              {mode === 'login'
                ? '使用账号进入广告决策工作台。'
                : 'Demo 新账号默认拥有管理员权限。'}
            </p>
          </div>
          {checking ? (
            <p className="flex items-center gap-2 text-sm">
              <LoaderCircle className="size-4 animate-spin" />
              正在检查登录状态…
            </p>
          ) : unavailable ? (
            <>
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
              <Button type="button" onClick={() => window.location.reload()}>
                重新连接
              </Button>
            </>
          ) : (
            <form onSubmit={login} className="space-y-4">
              <label
                className="block space-y-1 text-sm"
                htmlFor="login-username"
              >
                用户名
                <Input
                  id="login-username"
                  autoComplete="username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                  minLength={mode === 'register' ? 3 : 2}
                  maxLength={mode === 'register' ? 32 : 64}
                  disabled={busy}
                />
              </label>
              <label
                className="block space-y-1 text-sm"
                htmlFor="login-password"
              >
                密码
                <Input
                  id="login-password"
                  type="password"
                  autoComplete={
                    mode === 'register' ? 'new-password' : 'current-password'
                  }
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  minLength={8}
                  maxLength={mode === 'register' ? 72 : 256}
                  disabled={busy}
                />
              </label>
              {mode === 'register' && (
                <label
                  className="block space-y-1 text-sm"
                  htmlFor="register-confirm-password"
                >
                  确认密码
                  <Input
                    id="register-confirm-password"
                    type="password"
                    autoComplete="new-password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    required
                    disabled={busy}
                  />
                </label>
              )}
              {error && (
                <p role="alert" className="text-sm text-destructive">
                  {error}
                </p>
              )}
              <Button type="submit" className="w-full" disabled={busy}>
                {busy && <LoaderCircle className="size-4 animate-spin" />}
                {busy
                  ? mode === 'register'
                    ? '正在注册…'
                    : '正在登录…'
                  : mode === 'register'
                    ? '注册并进入工作台'
                    : '登录'}
              </Button>
              {canRegister && (
                <Button
                  type="button"
                  variant="ghost"
                  className="w-full"
                  disabled={busy}
                  onClick={() => {
                    const next = mode === 'login' ? 'register' : 'login';
                    setMode(next);
                    setError('');
                    setConfirmPassword('');
                    setUsername(
                      next === 'login' ? defaults.current.username : '',
                    );
                    setPassword(
                      next === 'login' ? defaults.current.password : '',
                    );
                  }}
                >
                  {mode === 'login'
                    ? '没有账号？注册账号'
                    : '已有账号？返回登录'}
                </Button>
              )}
            </form>
          )}
        </section>
      </div>
    </main>
  );
}
