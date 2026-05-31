import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { SearchAddon } from '@xterm/addon-search';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { getTicket, wsShareUrl, wsTicketUrl } from './api';

export type TerminalTheme = 'dark' | 'light';

export type SessionMode =
  | { kind: 'create'; shell: string; name?: string }
  | { kind: 'attach'; name: string }
  | { kind: 'share'; shareToken: string };

export interface TerminalSessionOptions {
  mode: SessionMode;
  token: string; // empty when joining via share only
  fontSize: number;
  theme: TerminalTheme;
  readOnly?: boolean; // disable client-side input dispatch (server enforces too)
  onTitle?: (title: string) => void;
  onReady?: (info: { name: string; shell: string; created: boolean; mode?: string }) => void;
  onEnded?: (reason: string) => void;
  onClose?: () => void;
}

const darkTheme = {
  background: '#0b0d10',
  foreground: '#d8e0e8',
  cursor: '#00d4aa',
  cursorAccent: '#0b0d10',
  selectionBackground: '#264f78',
  black: '#000000',
  red: '#ff5470',
  green: '#00d4aa',
  yellow: '#ffd866',
  blue: '#5ac8fa',
  magenta: '#c678dd',
  cyan: '#56b6c2',
  white: '#d8e0e8',
  brightBlack: '#5a6573',
  brightRed: '#ff7a8e',
  brightGreen: '#5fffbf',
  brightYellow: '#ffe49f',
  brightBlue: '#82d8ff',
  brightMagenta: '#d39bea',
  brightCyan: '#7ec9d2',
  brightWhite: '#ffffff',
};

const searchDecorations = {
  matchBackground: '#ffd866',
  matchBorder: '#ffd866',
  matchOverviewRuler: '#ffd866',
  activeMatchBackground: '#ff5470',
  activeMatchBorder: '#ff5470',
  activeMatchColorOverviewRuler: '#ff5470',
};

const lightTheme = {
  background: '#ffffff',
  foreground: '#1a1f25',
  cursor: '#00a385',
  cursorAccent: '#ffffff',
  selectionBackground: '#a6d3ff',
  black: '#1a1f25',
  red: '#d8344e',
  green: '#00a385',
  yellow: '#b08800',
  blue: '#0064c1',
  magenta: '#a626a4',
  cyan: '#0184bc',
  white: '#fafafa',
  brightBlack: '#5a6573',
  brightRed: '#ff5470',
  brightGreen: '#00c896',
  brightYellow: '#d4a300',
  brightBlue: '#3e8fdb',
  brightMagenta: '#c25fbf',
  brightCyan: '#23a7c0',
  brightWhite: '#ffffff',
};

export class TerminalSession {
  readonly element: HTMLDivElement;
  sessionName = '';
  shell = '';

  private term: Terminal;
  private fit: FitAddon;
  private search: SearchAddon;
  private ws: WebSocket | null = null;
  private resizeObserver: ResizeObserver | null = null;
  private closed = false;
  private ready = false;
  private opts: TerminalSessionOptions;
  private pendingInput: Uint8Array[] = [];

  constructor(opts: TerminalSessionOptions) {
    this.opts = opts;
    this.element = document.createElement('div');
    this.element.className = 'term-pane__inner';

    this.term = new Terminal({
      fontFamily: '"Cascadia Code", "JetBrains Mono", Menlo, Consolas, "Courier New", monospace',
      fontSize: opts.fontSize,
      cursorBlink: true,
      allowProposedApi: true,
      scrollback: 5000,
      theme: opts.theme === 'light' ? lightTheme : darkTheme,
    });
    this.fit = new FitAddon();
    this.search = new SearchAddon();
    this.term.loadAddon(this.fit);
    this.term.loadAddon(this.search);
    this.term.loadAddon(new WebLinksAddon());

    this.term.onTitleChange((t) => this.opts.onTitle?.(t));
    this.term.onData((data) => {
      if (this.opts.readOnly) return;
      this.sendInput(data);
    });
    this.term.onBinary((data) => {
      if (this.opts.readOnly) return;
      const bytes = new Uint8Array(data.length);
      for (let i = 0; i < data.length; i++) bytes[i] = data.charCodeAt(i) & 0xff;
      this.sendInputBinary(bytes);
    });
  }

  attach(parent: HTMLElement): void {
    parent.appendChild(this.element);
    this.term.open(this.element);
    this.scheduleFit();
    this.resizeObserver = new ResizeObserver(() => this.fitNow());
    this.resizeObserver.observe(this.element);
    void this.connect();
  }

  focus(): void {
    this.term.focus();
    this.scheduleFit();
  }

  /**
   * Fit after the browser has laid out (and painted) the pane. A single
   * synchronous fit right after open()/ready usually measures a pane that
   * isn't sized yet, locking the grid to ~1 column. On desktop you can
   * nudge it back by resizing the window; on mobile there's no such
   * gesture, so it stays a tiny unreadable column. Double-rAF guarantees
   * we measure the final laid-out size; the delayed retry covers slow
   * first layouts (e.g. webfont swap, mobile chrome settling).
   */
  private scheduleFit(): void {
    requestAnimationFrame(() => requestAnimationFrame(() => this.fitNow()));
    setTimeout(() => this.fitNow(), 150);
  }

  /**
   * Disconnect this tab. The server-side session keeps running unless
   * `kill` is true, in which case the shell is terminated.
   */
  dispose(opts: { kill?: boolean } = {}): void {
    if (this.closed) return;
    this.closed = true;
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      try {
        this.ws.send(JSON.stringify({ type: opts.kill ? 'kill' : 'detach' }));
      } catch { /* ignore */ }
    }
    if (this.ws) {
      try { this.ws.close(); } catch { /* ignore */ }
      this.ws = null;
    }
    this.element.remove();
    this.term.dispose();
    this.opts.onClose?.();
  }

  setTheme(theme: TerminalTheme): void {
    this.opts.theme = theme;
    this.term.options.theme = theme === 'light' ? lightTheme : darkTheme;
  }

  setFontSize(size: number): void {
    this.opts.fontSize = size;
    this.term.options.fontSize = size;
    this.fitNow();
  }

  setReadOnly(readOnly: boolean): void {
    this.opts.readOnly = readOnly;
  }

  searchNext(query: string): boolean {
    if (!query) return false;
    return this.search.findNext(query, { decorations: searchDecorations });
  }

  searchPrev(query: string): boolean {
    if (!query) return false;
    return this.search.findPrevious(query, { decorations: searchDecorations });
  }

  searchClear(): void {
    this.search.clearDecorations();
  }

  sendKey(data: string): void {
    this.sendInput(data);
    this.focus();
  }

  private fitNow(): void {
    // Never fit a zero-size (detached / display:none) element — doing so
    // would lock in a bogus ~1-column grid that only a later resize fixes.
    if (!this.element.clientWidth || !this.element.clientHeight) return;
    try { this.fit.fit(); } catch { /* element may be hidden */ }
    if (this.ready && this.ws?.readyState === WebSocket.OPEN) {
      const { cols, rows } = this.term;
      this.ws.send(JSON.stringify({ type: 'resize', cols, rows }));
    }
  }

  private async connect(): Promise<void> {
    let url: string;
    if (this.opts.mode.kind === 'share') {
      url = wsShareUrl(this.opts.mode.shareToken);
    } else {
      // Trade the long-lived token for a short-lived ticket so it never
      // appears in the WebSocket URL (which proxies log).
      try {
        const ticket = await getTicket(this.opts.token);
        if (this.closed) return;
        url = wsTicketUrl(ticket);
      } catch {
        this.term.write('\r\n\x1b[31m[connection error — could not authorize]\x1b[0m\r\n');
        return;
      }
    }
    const ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';
    this.ws = ws;

    ws.onopen = () => {
      const { cols, rows } = this.term;
      // Share mode auths via ?share= at the URL and skips the
      // create/attach handshake — the server attaches automatically.
      if (this.opts.mode.kind === 'share') {
        // Mark ready already so any pending input would be ignored
        // anyway (readOnly is set externally for viewer mode).
        return;
      }
      const handshake =
        this.opts.mode.kind === 'create'
          ? {
              type: 'create',
              shell: this.opts.mode.shell,
              name: this.opts.mode.name ?? '',
              cols,
              rows,
            }
          : { type: 'attach', name: this.opts.mode.name, cols, rows };
      ws.send(JSON.stringify(handshake));
    };

    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        try {
          const msg = JSON.parse(ev.data);
          if (msg.type === 'ready') {
            this.sessionName = msg.name;
            this.shell = msg.shell;
            this.ready = true;
            this.flushPending();
            this.scheduleFit();
            this.opts.onReady?.({
              name: msg.name,
              shell: msg.shell,
              created: !!msg.created,
              mode: msg.mode,
            });
          } else if (msg.type === 'ended') {
            this.opts.onEnded?.(msg.reason ?? 'session ended');
            this.term.write(`\r\n\x1b[90m[session ended: ${msg.reason ?? ''}]\x1b[0m\r\n`);
          } else if (msg.type === 'error') {
            this.term.write(`\r\n\x1b[31m[conduit] ${msg.message}\x1b[0m\r\n`);
          }
        } catch {
          this.term.write(ev.data);
        }
        return;
      }
      const data = new Uint8Array(ev.data as ArrayBuffer);
      this.term.write(data);
    };

    ws.onclose = () => {
      this.ready = false;
      if (!this.closed) {
        this.term.write('\r\n\x1b[90m[disconnected — close tab to detach]\x1b[0m\r\n');
      }
    };

    ws.onerror = () => {
      this.term.write('\r\n\x1b[31m[connection error]\x1b[0m\r\n');
    };
  }

  private sendInput(data: string): void {
    const bytes = new TextEncoder().encode(data);
    this.sendInputBinary(bytes);
  }

  private sendInputBinary(data: Uint8Array): void {
    if (!this.ready || this.ws?.readyState !== WebSocket.OPEN) {
      this.pendingInput.push(data);
      return;
    }
    this.ws.send(data);
  }

  private flushPending(): void {
    if (!this.ws) return;
    for (const buf of this.pendingInput) {
      this.ws.send(buf);
    }
    this.pendingInput = [];
  }
}
