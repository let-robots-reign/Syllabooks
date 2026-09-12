// cx joins class names, skipping falsy ones: cx(styles.box, bad && styles.bad).
export function cx(...names: (string | false | null | undefined)[]): string {
  return names.filter(Boolean).join(" ");
}
