import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
} from "react";
import { createPortal } from "react-dom";
import clsx from "clsx";
import styles from "./Modal.module.scss";

const focusable = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

// Modal is a window over the page: Escape, the backdrop and the cross close
// it, Tab stays inside, the page behind does not scroll, and focus goes back
// to whatever opened it. It stays mounted through its closing animation.
export function Modal({
  open,
  onClose,
  title,
  initialFocusRef,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  // Receives focus on open; otherwise the first focusable element does.
  initialFocusRef?: RefObject<HTMLElement | null>;
  children: ReactNode;
}) {
  const titleId = useId();
  const panelRef = useRef<HTMLDivElement>(null);
  const pressedOnBackdrop = useRef(false);
  const [rendered, setRendered] = useState(open);
  if (open && !rendered) setRendered(true);
  const closing = rendered && !open;

  // Remember what opened the modal, and give focus back when it closes.
  useEffect(() => {
    if (!open) return;
    const opener = document.activeElement as HTMLElement | null;
    const target =
      initialFocusRef?.current ??
      panelRef.current?.querySelector<HTMLElement>(focusable) ??
      panelRef.current;
    target?.focus();
    return () => opener?.focus();
  }, [open, initialFocusRef]);

  useEffect(() => {
    if (!rendered) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, [rendered]);

  if (!rendered) return null;

  const trapFocus = (event: KeyboardEvent) => {
    if (event.key === "Escape") {
      event.stopPropagation();
      onClose();
      return;
    }
    if (event.key !== "Tab" || !panelRef.current) return;
    const items = [
      ...panelRef.current.querySelectorAll<HTMLElement>(focusable),
    ];
    if (items.length === 0) {
      event.preventDefault();
      return;
    }
    const first = items[0];
    const last = items[items.length - 1];
    const active = document.activeElement;
    if (event.shiftKey && (active === first || active === panelRef.current)) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && active === last) {
      event.preventDefault();
      first.focus();
    }
  };

  return createPortal(
    <div
      className={clsx(styles.backdrop, closing && styles.closing)}
      onPointerDown={(event) => {
        pressedOnBackdrop.current = event.target === event.currentTarget;
      }}
      onClick={(event) => {
        // Only a click that starts and ends on the backdrop closes: selecting
        // text in the field and releasing outside must not.
        if (pressedOnBackdrop.current && event.target === event.currentTarget) {
          onClose();
        }
        pressedOnBackdrop.current = false;
      }}
    >
      <div
        ref={panelRef}
        className={styles.panel}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        onKeyDown={trapFocus}
        onAnimationEnd={(event) => {
          if (closing && event.target === event.currentTarget) {
            setRendered(false);
          }
        }}
      >
        <header className={styles.header}>
          <h2 id={titleId} className={styles.title}>
            {title}
          </h2>
          <button
            type="button"
            className={styles.close}
            aria-label="Закрыть"
            onClick={onClose}
          >
            <svg viewBox="0 0 16 16" aria-hidden="true">
              <path d="M3 3l10 10M13 3L3 13" />
            </svg>
          </button>
        </header>
        <div className={styles.body}>{children}</div>
      </div>
    </div>,
    document.body,
  );
}
