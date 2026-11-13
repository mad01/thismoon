// a11y.ts — the keyboard path's decisions, kept apart from the DOM wiring in
// webkit.ts so node can test them on stand-in elements: which wk-title is the
// page heading, which filter badge webkit makes keyboard-operable, which keys
// press a role="button", and where the header's skip link lands.

/** Containers whose wk-title names the card, not the page. */
export const TITLE_CONTAINERS = 'wk-card, .wk-card, wk-panel';

interface Attributes {
  hasAttribute(name: string): boolean;
  getAttribute(name: string): string | null;
}

/** Whether a wk-title should announce itself as the page's level-1 heading:
 * it carries no role of its own and sits outside a card or panel. */
export function wantsPageHeading(title: Attributes & { closest(selector: string): unknown }): boolean {
  return !title.hasAttribute('role') && title.closest(TITLE_CONTAINERS) === null;
}

/** Ancestors that already take the keyboard; a badge inside one stays a label. */
export const INTERACTIVE_ANCESTORS = 'a, button';

/** Whether webkit should make a badge keyboard-operable: a filter badge the
 * page left without a tabindex or a role, outside any link or button. A badge
 * that sets either one is the page's to wire (csl and moondb do), and one in a
 * link or button would nest a button in a control that already has the key. */
export function wantsFilterKeys(badge: Attributes & { closest(selector: string): unknown }): boolean {
  return badge.getAttribute('variant') === 'filter' &&
    !badge.hasAttribute('tabindex') && !badge.hasAttribute('role') &&
    badge.closest(INTERACTIVE_ANCESTORS) === null;
}

/** Whether a key presses a role="button" element, as Enter and Space press a
 * native button. */
export function isActivationKey(key: string): boolean {
  return key === 'Enter' || key === ' ';
}

/** Where the header's skip link moves focus: the first main, else the .wrap
 * container, else whatever follows the header. */
export function skipTarget<T>(
  doc: { querySelector(selector: string): T | null },
  header: { nextElementSibling: T | null },
): T | null {
  return doc.querySelector('main') ?? doc.querySelector('.wrap') ?? header.nextElementSibling;
}
