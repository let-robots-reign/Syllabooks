import type { ReactNode } from "react";
import styles from "./Card.module.scss";
import { cx } from "./cx.ts";
import { Eyebrow } from "./Eyebrow.tsx";

type Props = {
  label?: string;
  className?: string;
  children: ReactNode;
};

// Card is the sand box that holds a record on the cream page: the loan slip
// ("Формуляр"), the book being returned.
export function Card({ label, className, children }: Props) {
  return (
    <div className={cx(styles.card, className)}>
      {label && <Eyebrow className={styles.label}>{label}</Eyebrow>}
      {children}
    </div>
  );
}
