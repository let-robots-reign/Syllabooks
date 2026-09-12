import type { ButtonHTMLAttributes, ComponentProps } from "react";
import styles from "./Button.module.scss";
import { cx } from "./cx.ts";
import { Link } from "./Link.tsx";

type Look = {
  // primary: ink, the one main action on a screen. secondary: outlined, an
  // alternative of equal weight (manual ISBN entry). brand: spine green, kept
  // for the finish celebration. quiet: a text-only row, e.g. "Позже".
  variant?: "primary" | "secondary" | "brand" | "quiet";
  // Shorter, with a smaller label: for actions inside a card or a state.
  compact?: boolean;
};

function lookClass({ variant = "primary", compact }: Look, className?: string) {
  return cx(
    styles.button,
    styles[variant],
    compact && styles.compact,
    className,
  );
}

// Buttons are full-width blocks; put two in a row with a flex wrapper.
export function Button({
  variant,
  compact,
  className,
  type = "button",
  ...rest
}: Look & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type={type}
      className={lookClass({ variant, compact }, className)}
      {...rest}
    />
  );
}

// ButtonLink is a link that looks like a Button.
export function ButtonLink({
  variant,
  compact,
  className,
  ...rest
}: Look & ComponentProps<typeof Link>) {
  return (
    <Link className={lookClass({ variant, compact }, className)} {...rest} />
  );
}
