import en from './en.json';

// Localization lookup. English is the reference locale; every visible
// string in the client comes from here, nothing is hard-coded in a
// component. Later locales will be additional JSON files with the same
// keys.

export type MessageKey = keyof typeof en;

export function t(key: MessageKey, vars?: Record<string, string | number>): string {
  const table = en as Record<string, string>;
  let text = table[key] ?? key;
  if (vars) {
    for (const [name, value] of Object.entries(vars)) {
      text = text.replaceAll(`{${name}}`, String(value));
    }
  }
  return text;
}
