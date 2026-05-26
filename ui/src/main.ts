import './style.css';
import {
  Leaf,
  RootBox,
  forEachLeaf,
  leafContaining,
  makeLeafEl,
  nextId,
  removeLeaf,
  splitLeaf,
} from './layout';
import { SessionMode, TerminalSession, TerminalTheme } from './terminal';
import { Toolbar } from './toolbar';
import {
  CreatedShare,
  FileEntry,
  Preset,
  ShareMode,
  SessionInfo,
  ShellInfo,
  clearToken,
  createShare,
  downloadFileUrl,
  fetchPresets,
  fetchSessions,
  fetchShells,
  getToken,
  killSession,
  launchPreset,
  listFiles,
  setToken,
  uploadFiles,
  verifyToken,
} from './api';
import { toast } from './toast';

const SETTINGS_KEY = 'conduit.settings';
const TABS_KEY = 'conduit.tabs';

interface Settings {
  theme: TerminalTheme;
  fontSize: number;
}

interface Tab {
  id: string;
  /** Server session name of the FIRST leaf (used for title + persistence). */
  sessionName: string;
  shell: string;
  title: string;
  /** Root of this tab's pane layout tree. */
  rootBox: RootBox;
  /** The currently-focused leaf within this tab. */
  activeLeafId: string;
  /** Outer container appended to bodyEl. Holds the root layout element. */
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
        const s = this.activeSession();
        if (s) s.sendKey(data);
      },
    });
    this.root.appendChild(this.toolbar.element);

    // The normal openTab path now handles share mode via makeLeaf
    // (defaulting to read-only, flipping in onReady when the server
    // signals writer). Split/close pane chrome is harmless here — share
    // guests don't have a Sessions panel, so any spawn attempt would
    // fail server-side anyway.
    this.openTab({ kind: 'share', shareToken }, '');
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
    actions.appendChild(this.makeIconBtn('📁', 'Files', () => this.openFilesPanel()));
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
        const s = this.activeSession();
        if (s) s.sendKey(data);
      },
    });

    this.root.appendChild(this.tabbarEl);
    this.root.appendChild(this.bodyEl);
    this.root.appendChild(this.toolbar.element);
    this.installDragDropUpload();
    this.installSearchOverlay();
    this.installPaneShortcuts();

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

  // ---------------- Tabs + Panes ----------------

  private openTab(mode: SessionMode, shellHint: string): Tab {
    const tabId = Math.random().toString(36).slice(2, 10);
    const initialTitle =
      mode.kind === 'create'
        ? (mode.name || mode.shell)
        : mode.kind === 'attach'
          ? mode.name
          : 'shared';

    const pane = document.createElement('div');
    pane.className = 'term-pane';
    this.bodyEl.appendChild(pane);

    const firstLeaf = this.makeLeaf(mode, shellHint, tabId, /* isFirst */ true, initialTitle);
    pane.appendChild(firstLeaf.el);

    const tabEl = document.createElement('div');
    tabEl.className = 'tab';
    tabEl.innerHTML = `<span class="tab__title">${escapeHtml(initialTitle)}</span><button class="tab__close" type="button" aria-label="detach">×</button>`;
    tabEl.addEventListener('click', (e) => {
      if ((e.target as HTMLElement).classList.contains('tab__close')) return;
      this.activateTab(tabId);
    });
    tabEl.querySelector('.tab__close')!.addEventListener('click', (e) => {
      e.stopPropagation();
      this.closeTab(tabId, { kill: false });
    });
    this.tabbarInnerEl.appendChild(tabEl);

    const tab: Tab = {
      id: tabId,
      sessionName: '',
      shell: shellHint,
      title: initialTitle,
      rootBox: { value: firstLeaf },
      activeLeafId: firstLeaf.id,
      pane,
      tabEl,
    };
    this.tabs.push(tab);
    this.activateTab(tabId);
    this.emptyEl.style.display = 'none';
    return tab;
  }

  /** Build a fresh Leaf for the given mode and wire its session callbacks. */
  private makeLeaf(
    mode: SessionMode,
    shellHint: string,
    tabId: string,
    isFirst: boolean,
    initialTitle: string,
  ): Leaf {
    const leafId = nextId('leaf');
    const el = makeLeafEl();
    // Pane chrome: tiny header with split/close buttons.
    const header = document.createElement('div');
    header.className = 'leaf__chrome';
    header.innerHTML = `
      <button type="button" class="leaf__btn" data-action="split-right" title="Split right (Ctrl+Shift+H)">⇥</button>
      <button type="button" class="leaf__btn" data-action="split-down"  title="Split down (Ctrl+Shift+V)">⇩</button>
      <button type="button" class="leaf__btn leaf__btn--danger" data-action="close" title="Close pane (Ctrl+Shift+W)">✕</button>
    `;
    const body = document.createElement('div');
    body.className = 'leaf__body';
    el.appendChild(header);
    el.appendChild(body);

    const isShare = mode.kind === 'share';
    const session = new TerminalSession({
      mode,
      token: this.token,
      fontSize: this.settings.fontSize,
      theme: this.settings.theme,
      // Share mode starts read-only — server announces viewer-vs-writer
      // via msg.mode on ready, and we flip if needed.
      readOnly: isShare,
      onTitle: (t) => {
        if (isFirst) this.setTabTitle(tabId, t || initialTitle);
      },
      onReady: ({ name, shell, mode: ackMode }) => {
        const tab = this.tabs.find((x) => x.id === tabId);
        const leaf = tab ? findLeafInTab(tab, leafId) : null;
        if (leaf) {
          leaf.shell = shell;
        }
        if (isShare && ackMode === 'writer') {
          session.setReadOnly(false);
        }
        if (isShare && isFirst && tab) {
          const label = ackMode === 'writer' ? `${name} (shared)` : `${name} (read-only)`;
          this.setTabTitle(tabId, label);
        }
        if (!isShare && isFirst && tab) {
          tab.sessionName = name;
          tab.shell = shell;
          if (!tab.title || tab.title === initialTitle) {
            this.setTabTitle(tabId, name);
          }
          this.persistTabs();
        }
      },
      onEnded: () => {
        if (isFirst) {
          const tab = this.tabs.find((x) => x.id === tabId);
          if (tab) tab.tabEl.classList.add('tab--ended');
        }
      },
      onClose: () => this.removeLeafFromTab(tabId, leafId),
    });
    session.attach(body);

    // Click anywhere in the pane → focus it.
    el.addEventListener('mousedown', () => this.setActiveLeaf(tabId, leafId));

    // Pane chrome actions.
    header.querySelector('[data-action="split-right"]')!.addEventListener('click', (e) => {
      e.stopPropagation();
      this.splitActive(tabId, 'horizontal', shellHint);
    });
    header.querySelector('[data-action="split-down"]')!.addEventListener('click', (e) => {
      e.stopPropagation();
      this.splitActive(tabId, 'vertical', shellHint);
    });
    header.querySelector('[data-action="close"]')!.addEventListener('click', (e) => {
      e.stopPropagation();
      this.closeActivePaneOrTab(tabId);
    });

    return { kind: 'leaf', id: leafId, shell: shellHint, session, el };
  }

  /**
   * Split the active leaf of the given tab. The new pane spawns a fresh
   * server-side session using the same shell as the source pane.
   */
  private splitActive(tabId: string, orientation: 'horizontal' | 'vertical', shellHint: string): void {
    const tab = this.tabs.find((x) => x.id === tabId);
    if (!tab) return;
    const active = findLeafInTab(tab, tab.activeLeafId);
    if (!active) return;
    const shell = active.shell || shellHint || tab.shell;
    if (!shell) {
      toast('No shell to spawn — pick one in a new tab first.', 'error');
      return;
    }
    const newLeaf = this.makeLeaf({ kind: 'create', shell }, shell, tabId, /* isFirst */ false, shell);
    splitLeaf(tab.rootBox, active, newLeaf, orientation);
    this.setActiveLeaf(tabId, newLeaf.id);
  }

  /** Close the active pane. If it's the only pane in the tab, close the tab. */
  private closeActivePaneOrTab(tabId: string): void {
    const tab = this.tabs.find((x) => x.id === tabId);
    if (!tab) return;
    const active = findLeafInTab(tab, tab.activeLeafId);
    if (!active) return;
    if (tab.rootBox.value === active) {
      this.closeTab(tabId, { kill: false });
      return;
    }
    // Detach (don't kill) the server session for this pane.
    active.session.dispose({ kill: false });
    // onClose callback will call removeLeafFromTab.
  }

  /** Remove a leaf from a tab, collapsing the layout. */
  private removeLeafFromTab(tabId: string, leafId: string): void {
    const tab = this.tabs.find((x) => x.id === tabId);
    if (!tab) return;
    const active = findLeafInTab(tab, leafId);
    if (!active) return;
    const wasActive = tab.activeLeafId === leafId;
    const newRoot = removeLeaf(tab.rootBox, active);
    if (!newRoot) {
      this.removeTab(tabId);
      return;
    }
    // Pick a surviving leaf as the new active.
    if (wasActive) {
      let firstSurviving: Leaf | null = null;
      forEachLeaf(newRoot, (l) => {
        if (!firstSurviving) firstSurviving = l;
      });
      if (firstSurviving) this.setActiveLeaf(tabId, (firstSurviving as Leaf).id);
    }
    // Trigger xterm fits since pane geometry just changed.
    requestAnimationFrame(() => forEachLeaf(tab.rootBox.value, (l) => l.session.focus()));
  }

  private setActiveLeaf(tabId: string, leafId: string): void {
    const tab = this.tabs.find((x) => x.id === tabId);
    if (!tab) return;
    tab.activeLeafId = leafId;
    forEachLeaf(tab.rootBox.value, (l) => {
      l.el.classList.toggle('leaf--active', l.id === leafId);
    });
    const active = findLeafInTab(tab, leafId);
    if (active) requestAnimationFrame(() => active.session.focus());
  }

  private activateTab(id: string): void {
    this.activeTabId = id;
    for (const t of this.tabs) {
      const active = t.id === id;
      t.pane.classList.toggle('term-pane--active', active);
      t.tabEl.classList.toggle('tab--active', active);
      if (active) {
        const leaf = findLeafInTab(t, t.activeLeafId);
        if (leaf) requestAnimationFrame(() => leaf.session.focus());
      }
    }
  }

  private closeTab(id: string, opts: { kill: boolean }): void {
    const tab = this.tabs.find((x) => x.id === id);
    if (!tab) return;
    // Dispose all leaves' sessions; the last onClose will call removeTab.
    const leaves: Leaf[] = [];
    forEachLeaf(tab.rootBox.value, (l) => leaves.push(l));
    if (leaves.length === 0) {
      this.removeTab(id);
      return;
    }
    for (const l of leaves) {
      l.session.dispose({ kill: opts.kill });
    }
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

  /** Returns the TerminalSession of the active pane in the active tab. */
  private activeSession(): TerminalSession | null {
    const tab = this.activeTab();
    if (!tab) return null;
    const leaf = findLeafInTab(tab, tab.activeLeafId);
    return leaf ? leaf.session : null;
  }

  private forEachLeafInAllTabs(fn: (l: Leaf) => void): void {
    for (const t of this.tabs) forEachLeaf(t.rootBox.value, fn);
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
      <div class="settings__card settings__card--wide">
        <h2 class="settings__title">Sessions</h2>
        <div class="presets__wrap" hidden>
          <div class="presets__label">Presets</div>
          <div class="presets__list"></div>
        </div>
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

    // Presets section — only shown when the server has any configured.
    const presetsWrap = overlay.querySelector<HTMLDivElement>('.presets__wrap')!;
    const presetsList = overlay.querySelector<HTMLDivElement>('.presets__list')!;
    try {
      const presets = await fetchPresets(this.token);
      if (presets.length > 0) {
        presetsWrap.hidden = false;
        for (const p of presets) {
          presetsList.appendChild(this.renderPresetRow(p, overlay, refresh));
        }
      }
    } catch (e) {
      console.warn('fetchPresets failed', e);
    }
  }

  private renderPresetRow(preset: Preset, overlay: HTMLDivElement, refresh: () => Promise<void>): HTMLDivElement {
    const row = document.createElement('div');
    row.className = 'preset__row';
    const sessNames = preset.sessions.map((s) => s.name).join(', ');
    row.innerHTML = `
      <div class="preset__meta">
        <div class="preset__name">${escapeHtml(preset.name)}</div>
        <div class="preset__desc">${escapeHtml(preset.description ?? '')}</div>
        <div class="preset__sessions">${escapeHtml(sessNames)}</div>
      </div>
      <button type="button" class="sessions__btn" data-action="launch">Launch</button>
    `;
    row.querySelector('[data-action="launch"]')!.addEventListener('click', async () => {
      try {
        const res = await launchPreset(this.token, preset.name);
        let opened = 0;
        for (const r of res.launched) {
          const ps = preset.sessions.find((s) => s.name === r.session);
          if (!ps) continue;
          if (r.status === 'error') {
            toast(`${r.session}: ${r.error}`, 'error', 6000);
            continue;
          }
          // Open a tab attached to the (now-existing) server session.
          if (!this.tabs.find((t) => t.sessionName === r.session)) {
            this.openTab({ kind: 'attach', name: r.session }, ps.shell);
            opened++;
          }
        }
        toast(`Launched preset "${preset.name}" (${opened} new tab(s))`, 'success');
        overlay.remove();
        await refresh();
      } catch (e) {
        toast(`Launch failed: ${(e as Error).message}`, 'error', 6000);
      }
    });
    return row;
  }

  // ---------------- Pane keyboard shortcuts ----------------

  private installPaneShortcuts(): void {
    window.addEventListener('keydown', (e) => {
      if (!(e.ctrlKey || e.metaKey) || !e.shiftKey) return;
      const tab = this.activeTab();
      if (!tab) return;
      // Honor browser inputs (e.g. settings dialog text fields)
      const target = document.activeElement;
      const inOtherInput =
        target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement;
      if (inOtherInput && !(target.closest('.leaf') || target.closest('.term-pane'))) return;
      switch (e.key.toLowerCase()) {
        case 'h':
          e.preventDefault();
          this.splitActive(tab.id, 'horizontal', tab.shell);
          break;
        case 'v':
          e.preventDefault();
          this.splitActive(tab.id, 'vertical', tab.shell);
          break;
        case 'w':
          e.preventDefault();
          this.closeActivePaneOrTab(tab.id);
          break;
      }
    });
    // Also: click anywhere within any layout to focus the leaf under cursor.
    this.bodyEl.addEventListener('mousedown', (e) => {
      const tab = this.activeTab();
      if (!tab) return;
      const leaf = leafContaining(tab.rootBox.value, e.target);
      if (leaf) this.setActiveLeaf(tab.id, leaf.id);
    });
  }

  // ---------------- Search overlay (Ctrl-F) ----------------

  private installSearchOverlay(): void {
    const bar = document.createElement('div');
    bar.className = 'searchbar';
    bar.innerHTML = `
      <input type="search" class="searchbar__input" placeholder="Search scrollback…" />
      <button type="button" class="searchbar__btn" data-action="prev" title="Previous (Shift+Enter)">↑</button>
      <button type="button" class="searchbar__btn" data-action="next" title="Next (Enter)">↓</button>
      <button type="button" class="searchbar__btn" data-action="close" title="Close (Esc)">✕</button>
    `;
    this.bodyEl.appendChild(bar);
    const input = bar.querySelector<HTMLInputElement>('.searchbar__input')!;

    const open = () => {
      bar.classList.add('searchbar--visible');
      input.focus();
      input.select();
    };
    const close = () => {
      bar.classList.remove('searchbar--visible');
      const s = this.activeSession();
      s?.searchClear();
      s?.focus();
    };
    const next = () => this.activeSession()?.searchNext(input.value);
    const prev = () => this.activeSession()?.searchPrev(input.value);

    bar.querySelector('[data-action="next"]')!.addEventListener('click', () => next());
    bar.querySelector('[data-action="prev"]')!.addEventListener('click', () => prev());
    bar.querySelector('[data-action="close"]')!.addEventListener('click', () => close());

    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        if (e.shiftKey) prev();
        else next();
      } else if (e.key === 'Escape') {
        e.preventDefault();
        close();
      }
    });
    input.addEventListener('input', () => {
      // Live highlight as the user types.
      this.activeSession()?.searchNext(input.value);
    });

    window.addEventListener('keydown', (e) => {
      // Ctrl-F (or Cmd-F on Mac). Don't capture if focus is in an input
      // that isn't ours.
      const inSearch = document.activeElement === input;
      const inOtherInput =
        document.activeElement instanceof HTMLInputElement ||
        document.activeElement instanceof HTMLTextAreaElement;
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
        if (this.tabs.length === 0) return;
        if (inOtherInput && !inSearch) return; // don't hijack other inputs
        e.preventDefault();
        open();
      } else if (e.key === 'Escape' && bar.classList.contains('searchbar--visible')) {
        close();
      }
    });
  }

  // ---------------- Drag-drop upload ----------------

  private installDragDropUpload(): void {
    const overlay = document.createElement('div');
    overlay.className = 'dropzone';
    overlay.innerHTML = '<div class="dropzone__inner">Drop to upload to <code>files_root</code></div>';
    this.root.appendChild(overlay);

    let dragDepth = 0;
    window.addEventListener('dragenter', (e) => {
      if (!e.dataTransfer || !Array.from(e.dataTransfer.types).includes('Files')) return;
      e.preventDefault();
      dragDepth++;
      overlay.classList.add('dropzone--visible');
    });
    window.addEventListener('dragover', (e) => {
      if (!e.dataTransfer) return;
      if (Array.from(e.dataTransfer.types).includes('Files')) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'copy';
      }
    });
    window.addEventListener('dragleave', (e) => {
      if (!e.dataTransfer) return;
      dragDepth = Math.max(0, dragDepth - 1);
      if (dragDepth === 0) overlay.classList.remove('dropzone--visible');
    });
    window.addEventListener('drop', async (e) => {
      if (!e.dataTransfer || e.dataTransfer.files.length === 0) return;
      e.preventDefault();
      dragDepth = 0;
      overlay.classList.remove('dropzone--visible');
      try {
        const saved = await uploadFiles(this.token, e.dataTransfer.files);
        if (saved.length === 0) {
          toast('No files uploaded.', 'info');
        } else if (saved.length === 1) {
          toast(`Uploaded → ${saved[0].path}`, 'success');
        } else {
          toast(`Uploaded ${saved.length} files`, 'success');
        }
      } catch (err) {
        toast(`Upload failed: ${(err as Error).message}`, 'error', 6000);
      }
    });
  }

  // ---------------- Files panel ----------------

  private async openFilesPanel(): Promise<void> {
    let cwd = '';
    const overlay = document.createElement('div');
    overlay.className = 'settings';
    overlay.innerHTML = `
      <div class="settings__card settings__card--wide">
        <h2 class="settings__title">Files <span class="files__cwd"></span></h2>
        <p class="settings__sub">Rooted at the server's <code>files_root</code>. Drop files anywhere in the app to upload here.</p>
        <div class="files__toolbar">
          <button type="button" class="sessions__btn" data-action="up">⬆ Up</button>
          <button type="button" class="sessions__btn" data-action="refresh">Refresh</button>
          <label class="sessions__btn">Upload<input type="file" multiple hidden data-action="upload-input" /></label>
        </div>
        <div class="files__list">Loading…</div>
        <button class="settings__close" type="button" data-action="close">Close</button>
      </div>
    `;
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) overlay.remove();
    });
    overlay.querySelector('[data-action="close"]')!.addEventListener('click', () => overlay.remove());
    const cwdEl = overlay.querySelector<HTMLSpanElement>('.files__cwd')!;
    const listEl = overlay.querySelector<HTMLDivElement>('.files__list')!;

    const render = (entries: FileEntry[]) => {
      cwdEl.textContent = cwd ? '/ ' + cwd : '/';
      if (entries.length === 0) {
        listEl.innerHTML = '<div class="sessions__empty">Empty directory.</div>';
        return;
      }
      listEl.innerHTML = '';
      for (const e of entries) {
        const row = document.createElement('div');
        row.className = 'files__row';
        row.innerHTML = `
          <span class="files__icon">${e.dir ? '📂' : '📄'}</span>
          <span class="files__name">${escapeHtml(e.name)}</span>
          <span class="files__meta">${e.dir ? '' : humanSize(e.size)}</span>
        `;
        row.addEventListener('click', () => {
          if (e.dir) {
            cwd = e.path;
            refresh();
          } else {
            const a = document.createElement('a');
            a.href = downloadFileUrl(this.token, e.path);
            a.download = e.name;
            a.click();
          }
        });
        listEl.appendChild(row);
      }
    };
    const refresh = async () => {
      try {
        const listing = await listFiles(this.token, cwd);
        render(listing.entries ?? []);
      } catch (err) {
        listEl.innerHTML = `<div class="sessions__empty">${escapeHtml((err as Error).message)}</div>`;
      }
    };
    overlay.querySelector('[data-action="up"]')!.addEventListener('click', () => {
      if (!cwd) return;
      const parts = cwd.split('/').filter(Boolean);
      parts.pop();
      cwd = parts.join('/');
      refresh();
    });
    overlay.querySelector('[data-action="refresh"]')!.addEventListener('click', refresh);
    overlay.querySelector<HTMLInputElement>('[data-action="upload-input"]')!.addEventListener('change', async (e) => {
      const target = e.target as HTMLInputElement;
      if (!target.files || target.files.length === 0) return;
      try {
        const saved = await uploadFiles(this.token, target.files, cwd);
        toast(`Uploaded ${saved.length} file(s)`, 'success');
        target.value = '';
        await refresh();
      } catch (err) {
        toast(`Upload failed: ${(err as Error).message}`, 'error');
      }
    });

    this.root.appendChild(overlay);
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
        // Prefer the server's absolute_url (honors trust_proxy_headers
        // for tunnels / reverse proxies); fall back to location.origin.
        const fullURL = share.absolute_url || `${location.origin}${share.url}`;
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
      this.forEachLeafInAllTabs((l) => l.session.setTheme(v));
      sync();
    });
    overlay.querySelector('[data-group="fontSize"]')!.addEventListener('click', (e) => {
      const t = e.target as HTMLElement;
      const v = Number(t.dataset.value);
      if (!v) return;
      this.settings.fontSize = v;
      saveSettings(this.settings);
      this.forEachLeafInAllTabs((l) => l.session.setFontSize(v));
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

function findLeafInTab(tab: Tab, leafId: string): Leaf | null {
  let hit: Leaf | null = null;
  forEachLeaf(tab.rootBox.value, (l) => {
    if (l.id === leafId) hit = l;
  });
  return hit;
}

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let v = bytes / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 ? 0 : 1)} ${units[i]}`;
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

// PWA: register service worker for offline-first app shell. Skip during
// `vite dev` — the SW caches the dev shell and fights HMR.
if (!import.meta.env.DEV && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((err) => {
      console.warn('SW registration failed:', err);
    });
  });
}

const root = document.getElementById('app');
if (!root) throw new Error('missing #app');
new App(root).start().catch((e) => console.error(e));
