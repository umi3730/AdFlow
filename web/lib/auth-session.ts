export type Role = 'viewer' | 'operator' | 'admin';
export interface Principal {
  userId: string;
  username: string;
  role: Role;
}
export interface AuthInfo {
  authEnabled: boolean;
  principal: Principal;
}
export interface IssuedToken {
  accessToken: string;
  tokenType: string;
  expiresAt: string;
  principal: Principal;
}

export const roleLabels: Record<Role, string> = {
  viewer: '只读',
  operator: '运营',
  admin: '管理员',
};
export function roleAllows(role: Role | undefined, minimum: Role) {
  const rank = { viewer: 1, operator: 2, admin: 3 };
  return (role ? (rank[role] ?? 0) : 0) >= rank[minimum];
}

// Access tokens only live in this page's memory, never browser storage or URLs.
let token = '';
let expires = 0;
let revision = 0;
const listeners = new Set<() => void>();
function emit() {
  for (const listener of listeners) listener();
}
export function sessionRevision() {
  return revision;
}
export function sessionToken(now = Date.now()) {
  if (token && now >= expires) clearSession();
  return token;
}
export function sessionExpiry() {
  return expires;
}
export function setSession(issued: IssuedToken) {
  const expiry = Date.parse(issued.expiresAt);
  if (
    !issued.accessToken ||
    issued.tokenType !== 'Bearer' ||
    !Number.isFinite(expiry) ||
    expiry <= Date.now()
  )
    throw new Error('登录响应无效或已过期');
  token = issued.accessToken;
  expires = expiry;
  revision++;
  emit();
}
export function clearSession() {
  token = '';
  expires = 0;
  revision++;
  emit();
}
export function expireSessionIfCurrent(expected: number) {
  if (revision === expected) clearSession();
}
export function subscribeSession(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
