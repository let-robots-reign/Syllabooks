import clsx from "clsx";
import styles from "./Wordmark.module.scss";

type Props = {
  // Font size in px; every part of the mark scales with it.
  size: number;
  className?: string;
};

// The wordmark: "Syllabooks" with two book spines, green and brick, standing
// in for the "ll", on a shelf rule. Built from type and boxes rather than an
// image, so it stays sharp at any size.
export function Wordmark({ size, className }: Props) {
  return (
    <span
      className={clsx(styles.wordmark, className)}
      style={{ fontSize: size }}
      role="img"
      aria-label="Syllabooks"
    >
      <span>Sy</span>
      <span className={clsx(styles.spine, styles.green)} />
      <span className={clsx(styles.spine, styles.brick)} />
      <span>abooks</span>
      <span className={styles.shelf} />
    </span>
  );
}
