import type { ReactNode } from "react";
import styles from "./BottomAction.module.scss";

export function BottomAction({
  note,
  children,
}: {
  note?: string;
  children?: ReactNode;
}) {
  return (
    <footer className={styles.action}>
      {note && <p>{note}</p>}
      {children}
    </footer>
  );
}
