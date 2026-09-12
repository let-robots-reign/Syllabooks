import type { ReactNode } from "react";
import { cx } from "./cx.ts";
import styles from "./Eyebrow.module.scss";

type Props = {
  // h2 or h3 when the label heads a section; p when it only labels.
  as?: "p" | "h2" | "h3";
  className?: string;
  children: ReactNode;
};

// Eyebrow is the small uppercase label over a section: "Общий счёт класса",
// "Формуляр".
export function Eyebrow({ as: Tag = "p", className, children }: Props) {
  return <Tag className={cx(styles.eyebrow, className)}>{children}</Tag>;
}
