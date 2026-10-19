/**
 * Helpers for deciding whether a keyboard shortcut may fire for an event.
 *
 * Shortcuts are global listeners, so each one must first rule out every place
 * where the same key already means something else: typing, moving a cursor,
 * choosing from a list, or answering a dialog.
 */

const TEXT_ENTRY_SELECTOR =
  'input, textarea, select, [contenteditable=""], [contenteditable="true"], [role="textbox"], [role="combobox"], [role="searchbox"], [role="spinbutton"]';

const LAYER_SELECTOR = '[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]';

function asElement(target: EventTarget | null): Element | null {
  return target !== null && typeof (target as Element).closest === "function"
    ? (target as Element)
    : null;
}

/** True when the key press is going into a field the user is typing in or choosing from. */
export function isTextEntryTarget(target: EventTarget | null): boolean {
  return asElement(target)?.closest(TEXT_ENTRY_SELECTOR) != null;
}

/** True when focus is inside a dialog, menu or list box that owns the keyboard. */
export function isInsideLayer(target: EventTarget | null): boolean {
  return asElement(target)?.closest(LAYER_SELECTOR) != null;
}

/**
 * A key combination safe to treat as an app shortcut: not an auto-repeat, not
 * part of an IME composition, and not already handled by something else.
 */
export function isPlainShortcutEvent(e: KeyboardEvent): boolean {
  // keyCode 229 is what some browsers report for a key press an input method
  // is handling, without setting isComposing.
  return !e.repeat && !e.isComposing && e.keyCode !== 229 && !e.defaultPrevented;
}

/**
 * The letter of an Alt shortcut, from the physical key where the browser
 * reports it. On a Mac, Option+M types "µ", so e.key alone would never match.
 */
export function shortcutLetter(e: KeyboardEvent): string {
  if (/^Key[A-Z]$/.test(e.code)) return e.code.slice(3).toLowerCase();
  return e.key.toLowerCase();
}

const INTERACTIVE_SELECTOR = [
  "a[href]",
  "button",
  "input",
  "select",
  "textarea",
  "summary",
  "label",
  "[contenteditable]",
  '[tabindex]:not([tabindex="-1"])',
  ...[
    "button",
    "link",
    "radio",
    "radiogroup",
    "checkbox",
    "switch",
    "tab",
    "tablist",
    "slider",
    "option",
    "menuitem",
    "menuitemradio",
    "menuitemcheckbox",
  ].map((role) => `[role="${role}"]`),
].join(", ");

/**
 * True when a key press is aimed at the page itself rather than at a control:
 * the document body, or non-interactive content such as the question heading.
 *
 * Enter and the arrow keys already mean something on every control (activate a
 * button, move through radio options, move a text cursor), so the exam-wide
 * shortcuts built on them may only act here.
 */
export function isSurfaceTarget(target: EventTarget | null): boolean {
  const el = asElement(target);
  if (el === null) return true; // window or document
  if (el === document.body || el === document.documentElement) return true;
  return el.closest(INTERACTIVE_SELECTOR) === null && !isInsideLayer(el) && !isTextEntryTarget(el);
}

/** A key press with no Ctrl, Alt, Meta or Shift held. */
export function hasNoModifiers(e: KeyboardEvent): boolean {
  return !e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey;
}
