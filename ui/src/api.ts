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
  /** Relative URL: "/?share=…". Always present. */
  url: string;
  /** Absolute URL built from request host/proto (and X-Forwarded-* when trusted). */
  absolute_url?: string;
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

// ---------------- Files ----------------

export interface UploadedFile {
  name: string;
  path: string;
  size: number;
}

export interface FileEntry {
  name: string;
  path: string;
  size: number;
  dir: boolean;
  modified: string;
}

export interface FileListing {
  root: string;
  entries: FileEntry[];
}

export async function uploadFiles(token: string, files: FileList | File[], dir = ''): Promise<UploadedFile[]> {
  const fd = new FormData();
  for (const f of Array.from(files)) {
    fd.append('files', f, f.name);
  }
  const url = dir ? `/api/files?dir=${encodeURIComponent(dir)}` : '/api/files';
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'X-Auth-Token': token },
    body: fd,
  });
  if (!res.ok) throw new Error(`upload: ${res.status} ${await res.text()}`);
  return (await res.json()) ?? [];
}

export async function downloadFileUrl(token: string, path: string): Promise<string> {
  // Use a short-lived ticket rather than the long-lived token so the
  // credential in the URL is worthless seconds later (and not a useful
  // thing to find in a proxy log or browser history).
  const ticket = await getTicket(token);
  return `/api/files/download?path=${encodeURIComponent(path)}&ticket=${encodeURIComponent(ticket)}`;
}

export async function listFiles(token: string, dir = ''): Promise<FileListing> {
  const url = dir ? `/api/files/list?dir=${encodeURIComponent(dir)}` : '/api/files/list';
  const res = await fetch(url, { headers: { 'X-Auth-Token': token } });
  if (!res.ok) throw new Error(`list: ${res.status}`);
  return res.json();
}

// ---------------- Presets ----------------

export interface PresetSession {
  name: string;
  shell: string;
  command?: string;
  dir?: string;
}

export interface Preset {
  name: string;
  description?: string;
  sessions: PresetSession[];
}

export interface PresetLaunchResult {
  preset: string;
  launched: Array<{ session: string; status: 'created' | 'attached' | 'error'; error?: string }>;
}

export async function fetchPresets(token: string): Promise<Preset[]> {
  const res = await fetch('/api/presets', { headers: { 'X-Auth-Token': token } });
  if (!res.ok) throw new Error(`presets: ${res.status}`);
  return res.json();
}

export async function launchPreset(token: string, name: string): Promise<PresetLaunchResult> {
  const res = await fetch(`/api/presets/${encodeURIComponent(name)}/launch`, {
    method: 'POST',
    headers: { 'X-Auth-Token': token },
  });
  if (!res.ok) throw new Error(`launch: ${res.status} ${await res.text()}`);
  return res.json();
}

// ---------------- Tickets + token rotation ----------------

/**
 * Exchange the long-lived token (sent as a header) for a short-lived
 * ticket that may safely appear in a URL — used for the WebSocket
 * upgrade and download links, the only paths that can't carry a header.
 */
export async function getTicket(token: string): Promise<string> {
  const res = await fetch('/api/ticket', {
    method: 'POST',
    headers: { 'X-Auth-Token': token },
  });
  if (!res.ok) throw new Error(`ticket: ${res.status}`);
  const data = (await res.json()) as { ticket: string };
  return data.ticket;
}

/** Rotate the server's auth token. Returns the new token. All other
 *  clients holding the old token are logged out. */
export async function rotateToken(token: string): Promise<string> {
  const res = await fetch('/api/token/rotate', {
    method: 'POST',
    headers: { 'X-Auth-Token': token },
  });
  if (!res.ok) throw new Error(`rotate: ${res.status}`);
  const data = (await res.json()) as { token: string };
  return data.token;
}

// ---------------- WS URL helpers ----------------

export function wsTicketUrl(ticket: string): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}/ws?ticket=${encodeURIComponent(ticket)}`;
}

export function wsShareUrl(shareToken: string): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${location.host}/ws?share=${encodeURIComponent(shareToken)}`;
}
