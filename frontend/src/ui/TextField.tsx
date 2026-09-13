import { useId, type InputHTMLAttributes, type ReactNode } from "react";
import clsx from "clsx";
import styles from "./TextField.module.scss";

type Props = InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  // Keeps the label for screen readers only, where the screen's heading
  // already says what goes in the field.
  hideLabel?: boolean;
  // Help under the field. An error takes its place.
  hint?: ReactNode;
  error?: string | null;
  // Inside the box, after the text: e.g. a "показать" toggle.
  trailing?: ReactNode;
  // Large, letter-spaced Zilla Slab, for codes and passwords copied off paper.
  code?: boolean;
};

export function TextField({
  label,
  hideLabel,
  hint,
  error,
  trailing,
  code,
  className,
  id,
  ...input
}: Props) {
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const noteId = `${inputId}-note`;
  const note = error || hint;

  return (
    <div className={clsx(styles.field, className)}>
      <label
        className={clsx(styles.label, hideLabel && styles.hidden)}
        htmlFor={inputId}
      >
        {label}
      </label>
      <div className={clsx(styles.box, error && styles.invalid)}>
        <input
          id={inputId}
          className={clsx(styles.input, code && styles.code)}
          aria-invalid={error ? true : undefined}
          aria-describedby={note ? noteId : undefined}
          {...input}
        />
        {trailing}
      </div>
      {note && (
        <p
          id={noteId}
          className={error ? styles.error : styles.hint}
          role={error ? "alert" : undefined}
        >
          {note}
        </p>
      )}
    </div>
  );
}
