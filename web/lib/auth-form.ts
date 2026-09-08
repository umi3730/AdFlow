export function demoLoginDefaults(enabled: boolean) {
  return enabled
    ? { username: 'admin', password: 'adflow-admin' }
    : { username: '', password: '' };
}

export function registrationError(
  username: string,
  password: string,
  confirm: string,
) {
  if (!/^[a-z0-9][a-z0-9_-]{2,31}$/i.test(username.trim()))
    return '用户名需为 3–32 位字母、数字、下划线或短横线，以字母或数字开头';
  if (
    Array.from(password).length < 8 ||
    new TextEncoder().encode(password).length > 72
  )
    return '密码至少 8 个字符，且不超过 72 字节';
  if (password !== confirm) return '两次输入的密码不一致';
  return '';
}
