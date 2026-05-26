type Modifier = 'ctrl' | 'alt' | 'shift';

interface ToolbarOpts {
  onKey: (data: string) => void;
}

const SPECIAL_KEYS: Array<{ label: string; data: string }> = [
  { label: 'ESC', data: '\x1b' },
  { label: 'TAB', data: '\t' },
  { label: '↑', data: '\x1b[A' },
  { label: '↓', data: '\x1b[B' },
  { label: '←', data: '\x1b[D' },
  { label: '→', data: '\x1b[C' },
  { label: 'HOME', data: '\x1b[H' },
  { label: 'END', data: '\x1b[F' },
  { label: 'PGUP', data: '\x1b[5~' },
  { label: 'PGDN', data: '\x1b[6~' },
  { label: '|', data: '|' },
  { label: '~', data: '~' },
  { label: '/', data: '/' },
];

const CTRL_KEYS = ['A', 'C', 'D', 'E', 'K', 'L', 'R', 'U', 'W', 'Z'];

export class Toolbar {
  readonly element: HTMLDivElement;
  private modifiers: Record<Modifier, boolean> = { ctrl: false, alt: false, shift: false };
  private opts: ToolbarOpts;

  constructor(opts: ToolbarOpts) {
    this.opts = opts;
    this.element = document.createElement('div');
    this.element.className = 'toolbar';
    this.render();
  }

  private render(): void {
    this.element.innerHTML = '';

    this.addKey('CTRL', () => this.toggleMod('ctrl'), 'mod-ctrl');
    this.addKey('ALT', () => this.toggleMod('alt'), 'mod-alt');

    for (const key of SPECIAL_KEYS) {
      this.addKey(key.label, () => this.send(key.data));
    }

    // Ctrl+letter shortcuts (always available, independent of CTRL toggle)
    for (const letter of CTRL_KEYS) {
      this.addKey(`^${letter}`, () => {
        const code = letter.charCodeAt(0) - 64; // ^A = 0x01
        this.send(String.fromCharCode(code));
      });
    }
  }

  private addKey(label: string, onPress: () => void, id?: string): void {
    const b = document.createElement('button');
    b.className = 'toolbar__key';
    b.type = 'button';
    b.textContent = label;
    if (id) b.dataset.id = id;
    b.addEventListener('click', (e) => {
      e.preventDefault();
      onPress();
    });
    this.element.appendChild(b);
  }

  private toggleMod(mod: Modifier): void {
    this.modifiers[mod] = !this.modifiers[mod];
    const btn = this.element.querySelector<HTMLButtonElement>(`[data-id="mod-${mod}"]`);
    btn?.setAttribute('aria-pressed', String(this.modifiers[mod]));
  }

  private send(data: string): void {
    let out = data;
    if (this.modifiers.alt && data.length === 1) {
      out = '\x1b' + data;
    }
    if (this.modifiers.ctrl && data.length === 1) {
      const c = data.toUpperCase().charCodeAt(0);
      if (c >= 64 && c <= 95) {
        out = String.fromCharCode(c - 64);
      }
    }
    this.opts.onKey(out);
    // Sticky modifiers reset after one use
    if (this.modifiers.alt) this.toggleMod('alt');
    if (this.modifiers.ctrl) this.toggleMod('ctrl');
  }
}
