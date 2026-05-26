import './style.css';
import { SessionMode, TerminalSession, TerminalTheme } from './terminal';
import { Toolbar } from './toolbar';
import {
  CreatedShare,
  ShareMode,
  SessionInfo,
  ShellInfo,
  clearToken,
  createShare,
  fetchSessions,
  fetchShells,
  getToken,
  killSession,
  setToken,
  verifyToken,
} from './api';

const SETTINGS_KEY = 'conduit.settings';
const TABS_KEY = 'conduit.tabs';

interface Settings {
  theme: TerminalTheme;
  fontSize: number;
}

interface Tab {
  id: string;
  sessionName: string;
  shell: string;
  title: string;
  session: TerminalSession;
  pane: HTMLDivElement;
  tabEl: HTMLDivElement;
}

interface StoredTab {
  sessionName: string;
}

class App {
  private root: HTMLElement;
  private token = '';
  private shells: ShellInfo[] = [];
  private tabs: Tab[] = [];
  private activeTabId: string | null = null;
  private settings: Settings;
  private bodyEl!: HTMLDivElement;
  private tabbarEl!: HTMLDivElement;
  private tabbarInnerEl!: HTMLDivElement;
  private toolbar!: Toolbar;
  private emptyEl!: HTMLDivElement;

  constructor(root: HTMLElement) {
    this.root = root;
    this.settings = loadSettings();
    applyTheme(this.settings.theme);
  }

  async start(): Promise<void> {
    // Share magic link: ?share=… → join as guest with no main token.
    // The single tab is read-only or read-write depending on the share's
    // mode (server enforces; UI reflects).
    const params = new URLSearchParams(location.search);
    const shareToken = params.get('share');
    if (shareToken) {
      history.replaceState({}, '', location.pathname + location.hash);
      await this.bootShareGuest(shareToken);
      return;
    }
    // QR / magic link: ?token=… → verify and store, strip from history.
    const urlToken = params.get('token');
    if (urlToken) {
      history.replaceState({}, '', location.pathname + location.hash);
      if (await verifyToken(urlToken)) {
        setToken(urlToken);
        this.token = urlToken;
        await this.bootApp();
        return;
      }
    }
    const existing = getToken();
    if (existing && (await verifyToken(existing))) {
      this.token = existing;
      await this.bootApp();
      return;
    }
    this.renderLogin();
  }

  // ---------------- Share guest (no main token) ----------------

  private async bootShareGuest(shareToken: string): Promise<void> {
    this.root.innerHTML = '';

    this.tabbarEl = document.createElement('div');
    this.tabbarEl.className = 'tabbar';
    this.tabbarInnerEl = document.createElement('div');
    this.tabbarInnerEl.style.display = 'flex';
    this.tabbarInnerEl.style.flex = '1';
    this.tabbarEl.appendChild(this.tabbarInnerEl);
    this.root.appendChild(this.tabbarEl);

    this.bodyEl = document.createElement('div');
    this.bodyEl.className = 'body';
    this.emptyEl = document.createElement('div');
    this.emptyEl.className = 'empty';
    this.bodyEl.appendChild(this.emptyEl);
    this.root.appendChild(this.bodyEl);

    this.toolbar = new Toolbar({
      onKey: (data) => {
        const tab = this.activeTab();
        if (tab) tab.session.sendKey(data);
      },
    });
    this.root.appendChild(this.toolbar.element);

    // Open a single tab in share mode. Start as read-only; will flip to
    // writable once the server confirms mode in onReady.
    const id = Math.random().toString(36).slice(2, 10);
    const pane = document.createElement('div');
    pane.className = 'term-pane';
    this.bodyEl.appendChild(pane);

    const session = new TerminalSession({
      mode: { kind: 'share', shareToken },
      token: '',
      fontSize: this.settings.fontSize,
      theme: this.settings.theme,
      readOnly: true,
      onTitle: (t) => this.setTabTitle(id, t || 'shared'),
      onReady: ({ name, shell, mode }) => {
        const t = this.tabs.find((x) => x.id === id);
        if (!t) return;
        t.sessionName = name;
        t.shell = shell;
        const label = mode === 'writer' ? `${name} (shared)` : `${name} (read-only)`;
        this.setTabTitle(id, label);
        // Server told us viewer-vs-writer; flip readOnly to match.
        if (mode === 'writer') {
          session.setReadOnly(false);
        }
      },
      onEnded: () => {
        const t = this.tabs.find((x) => x.id === id);
        if (t) t.tabEl.classList.add('tab--ended');
      },
      onClose: () => this.removeTab(id),
    });
    session.attach(pane);

    const tabEl = document.createElement('div');
    tabEl.className = 'tab tab--active';
    tabEl.innerHTML = `<span class="tab__title">shared</span>`;
    this.tabbarInnerEl.appendChild(tabEl);
    pane.classList.add('term-pane--active');

    const tab: Tab = { id, sessionName: '', shell: '', title: 'shared', session, pane, tabEl };
    this.tabs.push(tab);
    this.activeTabId = id;
    this.emptyEl.style.display = 'none';
    requestAnimationFrame(() => session.focus());
  }

  // ---------------- Login ----------------

  private renderLogin(): void {
    this.root.innerHTML = '';
    const wrap = document.createElement('div');
    wrap.className = 'login';
    wrap.innerHTML = `
      <div class="login__logo">C</div>
      <div>
        <h1 class="login__title">Conduit</h1>
        <p class="login__sub">Sign in with your access token</p>
      </div>
      <form class="login__form">
        <input class="login__input" type="password" placeholder="access token" autocomplete="off" autocapitalize="off" autocorrect="off" spellcheck="false" />
        <button class="login__btn" type="submit">Connect</button>
        <div class="login__error"></div>
      </form>
    `;
    const form = wrap.querySelector<HTMLFormElement>('.login__form')!;
    const input = wrap.querySelector<HTMLInputElement>('.login__input')!;
    const btn = wrap.querySelector<HTMLButtonElement>('.login__btn')!;
    const err = wrap.querySelector<HTMLDivElement>('.login__error')!;

    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const token = input.value.trim();
      if (!token) return;
      btn.disabled = true;
      err.textContent = '';
      try {
        const ok = await verifyToken(token);
        if (!ok) {
          err.textContent = 'Invalid token';
          btn.disabled = false;
          return;
        }
        setToken(token);
        this.token = token;
        await this.bootApp();
      } catch (e: unknown) {
        err.textContent = 'Connection failed';
        btn.disabled = false;
        console.error(e);
      }
    });

    this.root.appendChild(wrap);
    input.focus();
  }

  // ---------------- App shell ----------------

  private async bootApp(): Promise<void> {
    try {
      this.shells = await fetchShells(this.token);
    } catch (e) {
      console.error('fetchShells failed', e);
      clearToken();
      this.renderLogin();
      return;
    }

    this.root.innerHTML = '';

    // Tab bar
    this.tabbarEl = document.createElement('div');
    this.tabbarEl.className = 'tabbar';
    this.tabbarInnerEl = document.createElement('div');
    this.tabbarInnerEl.style.display = 'flex';
    this.tabbarInnerEl.style.flex = '1';
    this.tabbarInnerEl.style.minWidth = '0';
    this.tabbarEl.appendChild(this.tabbarInnerEl);

    const actions = document.createElement('div');
    actions.className = 'tabbar__actions';
    actions.appendChild(this.makeIconBtn('+', 'New session', () => this.openNewSessionDialog()));
    actions.appendChild(this.makeIconBtn('☰', 'Sessions', () => this.openSessionsPanel()));
    actions.appendChild(this.makeIconBtn('⚙', 'Settings', () => this.openSettings()));
    this.tabbarEl.appendChild(actions);

    // Body
    this.bodyEl = document.createElement('div');
    this.bodyEl.className = 'body';

    this.emptyEl = document.createElement('div');
    this.emptyEl.className = 'empty';
    this.emptyEl.textContent = 'No active sessions. Click + to start one.';
    this.bodyEl.appendChild(this.emptyEl);

    // Mobile toolbar
    this.toolbar = new Toolbar({
      onKey: (data) => {
        const tab = this.activeTab();
        if (tab) tab.session.sendKey(data);
      },
    });

    this.root.appendChild(this.tabbarEl);
    this.root.appendChild(this.bodyEl);
    this.root.appendChild(this.toolbar.element);

    // Restore previously-open tabs whose server sessions are still alive.
    await this.restoreTabs();

    if (this.tabs.length === 0) {
      if (this.shells.length === 0) {
        this.emptyEl.textContent = 'No shells detected on the server.';
      } else {
        this.openNewSessionDialog();
      }
    }
  }

  private makeIconBtn(label: string, title: string, onClick: () => void): HTMLButtonElement {
    const b = document.createElement('button');
    b.className = 'iconbtn';
    b.type = 'button';
    b.title = title;
    b.setAttribute('aria-label', title);
    b.textContent = label;
    b.addEventListener('click', onClick);
    return b;
  }

  // ---------------- Tab restoration ----------------

  private async restoreTabs(): Promise<void> {
    const stored = loadStoredTabs();
    if (stored.length === 0) return;
    let sessions: SessionInfo[];
    try {
      sessions = await fetchSessions(this.token);
    } catch (e) {
      console.warn('fetchSessions failed during restore', e);
      return;
    }
    const alive = new Map(sessions.filter((s) => s.alive).map((s) => [s.name, s]));
    const surviving: StoredTab[] = [];
    for (const t of stored) {
      const info = alive.get(t.sessionName);
      if (info) {
        this.openTab({ kind: 'attach', name: info.name }, info.shell);
        surviving.push(t);
      }
    }
    saveStoredTabs(surviving);
  }

  // ---------------- Tabs ----------------

  private openTab(mode: SessionMode, shellHint: string): Tab {
    const id = Math.random().toString(36).slice(2, 10);
    const initialTitle =
      mode.kind === 'create'
        ? (mode.name || mode.shell)
        : mode.kind === 'attach'
          ? mode.name
          : 'shared';

    const pane = document.createElement('div');
    pane.className = 'term-pane';
    this.bodyEl.appendChild(pane);

    const session = new TerminalSession({
      mode,
      token: this.token,
      fontSize: this.settings.fontSize,
      theme: this.settings.theme,
      onTitle: (t) => this.setTabTitle(id, t || initialTitle),
      onReady: ({ name, shell }) => {
        const t = this.tabs.find((x) => x.id === id);
        if (!t) return;
        t.sessionName = name;
        t.shell = shell;
        if (!t.title || t.title === initialTitle) {
          this.setTabTitle(id, name);
        }
        this.persistTabs();
      },
      onEnded: () => {
        // Session terminated on the server side. Mark tab visually.
        const t = this.tabs.find((x) => x.id === id);
        if (t) {
          t.tabEl.classList.add('tab--ended');
          this.persistTabs();
        }
      },
      onClose: () => this.removeTab(id),
    });
    session.attach(pane);

    const tabEl = document.createElement('div');
    tabEl.className = 'tab';
    tabEl.innerHTML = `<span class="tab__title">${escapeHtml(initialTitle)}</span><button class="tab__close" type="button" aria-label="detach">×</button>`;
    tabEl.addEventListener('click', (e) => {
      if ((e.target as HTMLElement).classList.contains('tab__close')) return;
      this.activateTab(id);
    });
    tabEl.querySelector('.tab__close')!.addEventListener('click', (e) => {
      e.stopPropagation();
      this.closeTab(id, { kill: false });
    });
    this.tabbarInnerEl.appendChild(tabEl);

    const tab: Tab = { id, sessionName: '', shell: shellHint, title: initialTitle, session, pane, tabEl };
    this.tabs.push(tab);
    this.activateTab(id);
    this.emptyEl.style.display = 'none';
    return tab;
  }

  private activateTab(id: string): void {
    this.activeTabId = id;
    for (const t of this.tabs) {
      const active = t.id === id;
      t.pane.classList.toggle('term-pane--active', active);
      t.tabEl.classList.toggle('tab--active', active);
      if (active) {
        requestAnimationFrame(() => t.session.focus());
      }
    }
  }

  private closeTab(id: string, opts: { kill: boolean }): void {
    const t = this.tabs.find((x) => x.id === id);
    if (!t) return;
    t.session.dispose({ kill: opts.kill });
  }

  private removeTab(id: string): void {
    const idx = this.tabs.findIndex((x) => x.id === id);
    if (idx < 0) return;
    const [t] = this.tabs.splice(idx, 1);
    t.tabEl.remove();
    t.pane.remove();
    if (this.activeTabId === id) {
      const next = this.tabs[idx] ?? this.tabs[idx - 1];
      if (next) {
        this.activateTab(next.id);
      } else {
        this.activeTabId = null;
        this.emptyEl.style.display = 'grid';
      }
    }
    this.persistTabs();
  }

  private setTabTitle(id: string, title: string): void {
    const t = this.tabs.find((x) => x.id === id);
    if (!t) return;
    t.title = title;
    const titleEl = t.tabEl.querySelector<HTMLSpanElement>('.tab__title');
    if (titleEl) titleEl.textContent = title;
  }

  private activeTab(): Tab | undefined {
    return this.tabs.find((t) => t.id === this.activeTabId);
  }

  private persistTabs(): void {
    const stored = this.tabs
      .filter((t) => t.sessionName)
      .map((t) => ({ sessionName: t.sessionName }));
    saveStoredTabs(stored);
  }

  // ---------------- New session dialog ----------------

  private openNewSessionDialog(): void {
    if (this.shells.length === 0) return;
    const overlay = document.createElement('div');
    overlay.className = 'picker';
    const shellOptions = this.shells
      .map((s) => `<option value="${escapeHtml(s.name)}">${escapeHtml(s.name)}</option>`)
      .join('');
    overlay.innerHTML = `
      <div class="picker__card">
        <h2 class="picker__title">New session</h2>
        <form class="picker__form">
          <label class="picker__label">
            <span>Shell</span>
            <select class="picker__select" name="shell">${shellOptions}</select>
          </label>
          <label class="picker__label">
            <span>Name <em>(optional)</em></span>
            <input class="picker__input" name="name" placeholder="auto-generated" autocomplete="off" />
          </label>
          <div class="picker__actions">
            <button type="button" class="picker__cancel">Cancel</button>
            <button type="submit" class="picker__submit">Start</button>
          </div>
        </form>
      </div>
    `;

    const form = overlay.querySelector<HTMLFormElement>('.picker__form')!;
    const select = form.querySelector<HTMLSelectElement>('select[name="shell"]')!;
    const nameInput = form.querySelector<HTMLInputElement>('input[name="name"]')!;

    form.addEventListener('submit', (e) => {
      e.preventDefault();
      const shell = select.value;
      const name = nameInput.value.trim();
      overlay.remove();
      this.openTab({ kind: 'create', shell, name: name || undefined }, shell);
    });
    overlay.querySelector<HTMLButtonElement>('.picker__cancel')!.addEventListener('click', () => overlay.remove());
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) overlay.remove();
    });

    this.root.appendChild(overlay);
    setTimeout(() => select.focus(), 0);
  }

  // ---------------- Sessions panel ----------------

  private async openSessionsPanel(): Promise<void> {
    const overlay = document.createElement('div');
    overlay.className = 'settings';
    overlay.innerHTML = `
      <div class="settings__card">
        <h2 class="settings__title">Sessions</h2>
        <div class="sessions__list">Loading…</div>
        <button class="settings__close" type="button" data-action="close">Close</button>
      </div>
    `;
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) overlay.remove();
    });
    overlay.querySelector('[data-action="close"]')!.addEventListener('click', () => overlay.remove());
    this.root.appendChild(overlay);

    const list = overlay.querySelector<HTMLDivElement>('.sessions__list')!;
    const render = (sessions: SessionInfo[]) => {
      if (sessions.length === 0) {
        list.innerHTML = '<div class="sessions__empty">No sessions on the server.</div>';
        return;
      }
      list.innerHTML = '';
      for (const s of sessions) {
        const openInTab = this.tabs.some((t) => t.sessionName === s.name);
        const row = document.createElement('div');
        row.className = 'sessions__row';
        row.innerHTML = `
          <div class="sessions__meta">
            <div class="sessions__name">${escapeHtml(s.name)}</div>
            <div class="sessions__sub">
              <span class="sessions__shell">${escapeHtml(s.shell)}</span>
              <span class="sessions__dot">·</span>
              <span class="sessions__attached">${s.attached} attached</span>
              <span class="sessions__dot">·</span>
              <span>${formatTime(s.created_at)}</span>
            </div>
          </div>
          <div class="sessions__actions">
            <button type="button" class="sessions__btn" data-action="${openInTab ? 'focus' : 'attach'}">${openInTab ? 'Focus' : 'Attach'}</button>
            <button type="button" class="sessions__btn" data-action="share">Share</button>
            <button type="button" class="sessions__btn sessions__btn--danger" data-action="kill">Kill</button>
          </div>
        `;
        row.querySelector('[data-action="attach"]')?.addEventListener('click', () => {
          overlay.remove();
          this.openTab({ kind: 'attach', name: s.name }, s.shell);
        });
        row.querySelector('[data-action="focus"]')?.addEventListener('click', () => {
          const t = this.tabs.find((x) => x.sessionName === s.name);
          if (t) {
            this.activateTab(t.id);
            overlay.remove();
          }
        });
        row.querySelector('[data-action="share"]')!.addEventListener('click', () => {
          this.openShareDialog(s.name);
        });
        row.querySelector('[data-action="kill"]')!.addEventListener('click', async () => {
          if (!confirm(`Kill session "${s.name}"? The shell process will terminate.`)) return;
          await killSession(this.token, s.name);
          await refresh();
        });
        list.appendChild(row);
      }
    };

    const refresh = async () => {
      try {
        const sessions = await fetchSessions(this.token);
        render(sessions);
      } catch (e) {
        list.innerHTML = '<div class="sessions__empty">Failed to load sessions.</div>';
        console.error(e);
      }
    };
    await refresh();
  }

  // ---------------- Share dialog ----------------

  private openShareDialog(sessionName: string): void {
    const overlay = document.createElement('div');
    overlay.className = 'settings';
    overlay.innerHTML = `
      <div class="settings__card">
        <h2 class="settings__title">Share "${escapeHtml(sessionName)}"</h2>
        <p class="settings__sub">Generate a time-limited link to give someone access to this session.</p>
        <div class="settings__row">
          <span class="settings__label">Mode</span>
          <div class="settings__seg" data-group="mode">
            <button type="button" data-value="viewer" aria-pressed="true">Read-only</button>
            <button type="button" data-value="writer">Can type</button>
          </div>
        </div>
        <div class="settings__row">
          <span class="settings__label">Expires</span>
          <div class="settings__seg" data-group="ttl">
            <button type="button" data-value="900">15m</button>
            <button type="button" data-value="3600" aria-pressed="true">1h</button>
            <button type="button" data-value="14400">4h</button>
            <button type="button" data-value="86400">24h</button>
          </div>
        </div>
        <button class="login__btn" type="button" data-action="create">Generate link</button>
        <div class="share__result" hidden>
          <div class="settings__label">Share URL — anyone with this link can join:</div>
          <input class="share__url" readonly />
          <div class="share__row">
            <button class="sessions__btn" type="button" data-action="copy">Copy</button>
            <button class="sessions__btn sessions__btn--danger" type="button" data-action="revoke">Revoke</button>
            <span class="share__expires"></span>
          </div>
        </div>
        <button class="settings__close" type="button" data-action="close">Close</button>
      </div>
    `;

    let mode: ShareMode = 'viewer';
    let ttl = 3600;
    const sync = () => {
      overlay.querySelectorAll<HTMLButtonElement>('[data-group="mode"] button').forEach((b) => {
        b.setAttribute('aria-pressed', String(b.dataset.value === mode));
      });
      overlay.querySelectorAll<HTMLButtonElement>('[data-group="ttl"] button').forEach((b) => {
        b.setAttribute('aria-pressed', String(Number(b.dataset.value) === ttl));
      });
    };
    overlay.querySelector('[data-group="mode"]')!.addEventListener('click', (e) => {
      const v = (e.target as HTMLElement).dataset.value;
      if (v === 'viewer' || v === 'writer') {
        mode = v;
        sync();
      }
    });
    overlay.querySelector('[data-group="ttl"]')!.addEventListener('click', (e) => {
      const v = Number((e.target as HTMLElement).dataset.value);
      if (v > 0) {
        ttl = v;
        sync();
      }
    });

    let lastShare: CreatedShare | null = null;
    const resultBox = overlay.querySelector<HTMLDivElement>('.share__result')!;
    const urlInput = overlay.querySelector<HTMLInputElement>('.share__url')!;
    const expiresEl = overlay.querySelector<HTMLSpanElement>('.share__expires')!;

    overlay.querySelector('[data-action="create"]')!.addEventListener('click', async () => {
      try {
        const share = await createShare(this.token, sessionName, mode, ttl);
        lastShare = share;
        const fullURL = `${location.origin}${share.url}`;
        urlInput.value = fullURL;
        expiresEl.textContent = `expires ${formatTime(share.expires_at)}`;
        resultBox.hidden = false;
        urlInput.select();
      } catch (e) {
        alert(`Failed to create share: ${(e as Error).message}`);
      }
    });
    overlay.querySelector('[data-action="copy"]')!.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(urlInput.value);
      } catch {
        urlInput.select();
        document.execCommand('copy');
      }
    });
    overlay.querySelector('[data-action="revoke"]')!.addEventListener('click', async () => {
      if (!lastShare) return;
      const ok = confirm('Revoke this share? The link will stop working immediately.');
      if (!ok) return;
      const res = await fetch(`/api/shares/${encodeURIComponent(lastShare.token)}`, {
        method: 'DELETE',
        headers: { 'X-Auth-Token': this.token },
      });
      if (res.ok) {
        resultBox.hidden = true;
        lastShare = null;
      }
    });
    overlay.querySelector('[data-action="close"]')!.addEventListener('click', () => overlay.remove());
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) overlay.remove();
    });
    this.root.appendChild(overlay);
  }

  // ---------------- Settings ----------------

  private openSettings(): void {
    const overlay = document.createElement('div');
    overlay.className = 'settings';
    overlay.innerHTML = `
      <div class="settings__card">
        <h2 class="settings__title">Settings</h2>
        <div class="settings__row">
          <span class="settings__label">Theme</span>
          <div class="settings__seg" data-group="theme">
            <button type="button" data-value="dark">Dark</button>
            <button type="button" data-value="light">Light</button>
          </div>
        </div>
        <div class="settings__row">
          <span class="settings__label">Font size</span>
          <div class="settings__seg" data-group="fontSize">
            <button type="button" data-value="12">12</button>
            <button type="button" data-value="14">14</button>
            <button type="button" data-value="16">16</button>
            <button type="button" data-value="18">18</button>
          </div>
        </div>
        <div class="settings__row" style="flex-direction: column; align-items: stretch; gap: 0.4rem;">
          <span class="settings__label">Access token</span>
          <div class="settings__token">${escapeHtml(this.token)}</div>
        </div>
        <button class="settings__close" type="button" data-action="logout">Sign out</button>
        <button class="settings__close" type="button" data-action="close">Close</button>
      </div>
    `;

    const sync = () => {
      overlay.querySelectorAll<HTMLButtonElement>('[data-group="theme"] button').forEach((b) => {
        b.setAttribute('aria-pressed', String(b.dataset.value === this.settings.theme));
      });
      overlay.querySelectorAll<HTMLButtonElement>('[data-group="fontSize"] button').forEach((b) => {
        b.setAttribute('aria-pressed', String(Number(b.dataset.value) === this.settings.fontSize));
      });
    };
    sync();

    overlay.querySelector('[data-group="theme"]')!.addEventListener('click', (e) => {
      const t = e.target as HTMLElement;
      const v = t.dataset.value as TerminalTheme | undefined;
      if (!v) return;
      this.settings.theme = v;
      saveSettings(this.settings);
      applyTheme(v);
      for (const tab of this.tabs) tab.session.setTheme(v);
      sync();
    });
    overlay.querySelector('[data-group="fontSize"]')!.addEventListener('click', (e) => {
      const t = e.target as HTMLElement;
      const v = Number(t.dataset.value);
      if (!v) return;
      this.settings.fontSize = v;
      saveSettings(this.settings);
      for (const tab of this.tabs) tab.session.setFontSize(v);
      sync();
    });
    overlay.querySelector('[data-action="close"]')!.addEventListener('click', () => overlay.remove());
    overlay.querySelector('[data-action="logout"]')!.addEventListener('click', () => {
      clearToken();
      localStorage.removeItem(TABS_KEY);
      for (const tab of [...this.tabs]) this.closeTab(tab.id, { kill: false });
      overlay.remove();
      location.reload();
    });
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) overlay.remove();
    });

    this.root.appendChild(overlay);
  }
}

function loadSettings(): Settings {
  try {
    const raw = localStorage.getItem(SETTINGS_KEY);
    if (raw) {
      const s = JSON.parse(raw) as Partial<Settings>;
      return {
        theme: s.theme === 'light' ? 'light' : 'dark',
        fontSize: typeof s.fontSize === 'number' ? s.fontSize : 14,
      };
    }
  } catch { /* ignore */ }
  return { theme: 'dark', fontSize: 14 };
}

function saveSettings(s: Settings): void {
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(s));
}

function loadStoredTabs(): StoredTab[] {
  try {
    const raw = localStorage.getItem(TABS_KEY);
    if (!raw) return [];
    const v = JSON.parse(raw);
    if (!Array.isArray(v)) return [];
    return v
      .filter((x): x is StoredTab => typeof x === 'object' && x && typeof x.sessionName === 'string')
      .map((x) => ({ sessionName: x.sessionName }));
  } catch {
    return [];
  }
}

function saveStoredTabs(tabs: StoredTab[]): void {
  localStorage.setItem(TABS_KEY, JSON.stringify(tabs));
}

function applyTheme(theme: TerminalTheme): void {
  document.documentElement.dataset.theme = theme;
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }[c] as string));
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    const diff = Math.floor((Date.now() - d.getTime()) / 1000);
    if (diff < 60) return `${diff}s ago`;
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    return d.toLocaleString();
  } catch {
    return iso;
  }
}

const root = document.getElementById('app');
if (!root) throw new Error('missing #app');
new App(root).start().catch((e) => console.error(e));
