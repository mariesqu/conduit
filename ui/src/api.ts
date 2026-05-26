export interface ShellInfo {
  name: string;
  path: string;
  args?: string[];
}

export interface SessionInfo {
  name: string;
  shell: string;
  created_at: string;
  attached: number;
  alive: boolean;
}

const TOKEN_KEY = 'conduit.token';

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export async function verifyToken(token: string): Promise<boolean> {
  const res = await fetch('/api/auth', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token }),
  });
  return res.ok;
}

export async function fetchShells(token: string): Promise<ShellInfo[]> {
  const res = await fetch('/api/shells', {
    headers: { 'X-Auth-Token': token },
  });
  if (!res.ok) throw new Error(`shells: ${res.status}`);
  return res.json();
}

export async function fetchSessions(token: string): Promise<SessionInfo[]> {
  const res = await fetch('/api/sessions', {
    headers: { 'X-Auth-Token': token },
  });
  if (!res.ok) throw new Error(`sessions: ${res.status}`);
  return res.json();
}

export async function killSession(token: string, name: string): Promise<boolean> {
  const res = await fetch(`/api/sessions/${encodeURIComponent(name)}`, {
    method: 'DELETE',
    headers: { 'X-Auth-Token': token },
  });
  return res.ok;
}

export type ShareMode = 'viewer' | 'writer';

export interface ShareInfo {
  token: string;
  session: string;
  mode: ShareMode;
  created_at: string;
  expires_at: string;
}

export interface CreatedShare {
  token: string;
  url: string;
  session: string;
  mode: ShareMode;
  created_at: string;
  expires_at: string;
}

export async function createShare(
  token: string,
  sessionName: string,
  mode: ShareMode,
  ttlSeconds: number,
): Promise<CreatedShare> {
  const res = await fetch(`/api/sessions/${encodeURIComponent(sessionName)}/share`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Auth-Token': token },
    body: JSON.stringify({ mode, ttl_seconds: ttlSeconds }),
  });
  if (!res.ok) throw new Error(`create share: ${res.status} ${await res.text()}`);
  return res.json();
}

export async function listShares(token: string): Promise<ShareInfo[]> {
  const res = await fetch('/api/shares', { headers: { 'X-Auth-Token': token } });
  if (!res.ok) throw new Error(`list shares: ${res.status}`);
  return res.json();
}

export async function revokeShare(token: string, shareToken: string): Promise<boolean> {
  const res = await fetch(`/api/shares/${encodeURIComponent(shareToken)}`, {
    method: 'DELETE',
    headers: { 'X-Auth-Token': token },
  });
  return res.ok;
}

export function wsUrl(token: string): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}/ws?token=${encodeURIComponent(token)}`;
}

export function wsShareUrl(shareToken: string): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}/ws?share=${encodeURIComponent(shareToken)}`;
}
