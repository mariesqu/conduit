// Binary layout tree of terminal panes within a tab.
//
// A Node is either:
//   • a Leaf  — wraps one TerminalSession in a chrome element
//   • a Split — two children (a, b), horizontal or vertical, 50/50 sized
//
// Mutations (splitLeaf, removeLeaf) update both the structure AND the DOM
// in lockstep. Leaf DOM elements are STABLE — we never detach an xterm
// from the document mid-mutation, because xterm loses its renderer state.

import { TerminalSession } from './terminal';

export type Orientation = 'horizontal' | 'vertical';

export interface Leaf {
  kind: 'leaf';
  id: string;
  shell: string;
  session: TerminalSession;
  el: HTMLDivElement; // pane chrome
}

export interface Split {
  kind: 'split';
  id: string;
  orientation: Orientation;
  a: Node;
  b: Node;
  el: HTMLDivElement;
}

export type Node = Leaf | Split;

/** RootBox lets callers mutate the root reference held by the tab. */
export interface RootBox {
  value: Node;
}

let idCounter = 0;
export function nextId(prefix: string): string {
  idCounter++;
  return `${prefix}-${idCounter.toString(36)}`;
}

/** Iterate every leaf in the tree in left-to-right / top-to-bottom order. */
export function forEachLeaf(node: Node, fn: (leaf: Leaf) => void): void {
  if (node.kind === 'leaf') {
    fn(node);
  } else {
    forEachLeaf(node.a, fn);
    forEachLeaf(node.b, fn);
  }
}

/** Find the leaf whose DOM contains `target`, if any. */
export function leafContaining(root: Node, target: EventTarget | null): Leaf | null {
  if (!(target instanceof globalThis.Node)) return null;
  let hit: Leaf | null = null;
  forEachLeaf(root, (l) => {
    if (!hit && l.el.contains(target as globalThis.Node)) hit = l;
  });
  return hit;
}

/** Locate the direct parent Split of `target`, or null if target is the root. */
function findParentSplit(root: Node, target: Node): Split | null {
  if (root.kind !== 'split') return null;
  if (root.a === target || root.b === target) return root;
  return findParentSplit(root.a, target) ?? findParentSplit(root.b, target);
}

/**
 * Split a Leaf into two siblings. The original leaf stays in its place
 * (a), and the new leaf becomes b. Returns the new Split.
 */
export function splitLeaf(
  box: RootBox,
  oldLeaf: Leaf,
  newLeaf: Leaf,
  orientation: Orientation,
): Split {
  const split: Split = {
    kind: 'split',
    id: nextId('split'),
    orientation,
    a: oldLeaf,
    b: newLeaf,
    el: makeSplitEl(orientation),
  };

  if (box.value === oldLeaf) {
    box.value = split;
  } else {
    const parent = findParentSplit(box.value, oldLeaf);
    if (!parent) throw new Error('splitLeaf: leaf not in tree');
    if (parent.a === oldLeaf) parent.a = split;
    else parent.b = split;
  }

  // DOM: replace oldLeaf.el with split.el, then put oldLeaf.el back
  // inside split.el alongside a divider and newLeaf.el.
  oldLeaf.el.replaceWith(split.el);
  split.el.appendChild(oldLeaf.el);
  split.el.appendChild(makeDivider(orientation));
  split.el.appendChild(newLeaf.el);

  return split;
}

/**
 * Remove a leaf from the tree. Returns:
 *   • the new root (which might be the surviving sibling promoted up), or
 *   • null if the leaf was the only one (caller should drop the tab).
 *
 * Caller is responsible for disposing the leaf's TerminalSession.
 */
export function removeLeaf(box: RootBox, target: Leaf): Node | null {
  if (box.value === target) {
    target.el.remove();
    return null;
  }
  const parent = findParentSplit(box.value, target);
  if (!parent) {
    return box.value; // not in tree — no-op
  }
  const sibling = parent.a === target ? parent.b : parent.a;
  const grandparent = findParentSplit(box.value, parent);

  if (!grandparent) {
    // parent IS the root → sibling becomes root
    box.value = sibling;
    parent.el.replaceWith(sibling.el);
  } else {
    if (grandparent.a === parent) grandparent.a = sibling;
    else grandparent.b = sibling;
    parent.el.replaceWith(sibling.el);
  }
  target.el.remove();
  return box.value;
}

/** Make a fresh split container element. */
export function makeSplitEl(orientation: Orientation): HTMLDivElement {
  const el = document.createElement('div');
  el.className = `split split--${orientation}`;
  return el;
}

function makeDivider(orientation: Orientation): HTMLDivElement {
  const d = document.createElement('div');
  d.className = `divider divider--${orientation}`;
  return d;
}

/** Make a fresh leaf-chrome element. The caller appends content. */
export function makeLeafEl(): HTMLDivElement {
  const el = document.createElement('div');
  el.className = 'leaf';
  el.tabIndex = -1;
  return el;
}
