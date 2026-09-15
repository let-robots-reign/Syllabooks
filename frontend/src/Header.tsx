import styles from "./Header.module.scss";
import { Link } from "./ui/Link.tsx";
import { Wordmark } from "./ui/Wordmark.tsx";

// Header tops the catalogue: the wordmark, and the reader's initial as the
// way to their profile. No avatars from OAuth (PRD §7).
export function Header({ name, isAdmin }: { name: string; isAdmin: boolean }) {
  return (
    <header className={styles.header}>
      <Link href="/" className={styles.home}>
        <Wordmark size={23} />
      </Link>
      <div className={styles.actions}>
        {isAdmin && (
          <Link href="/admin/books" className={styles.admin}>
            Админка
          </Link>
        )}
        <Link href="/profile" className={styles.avatar} aria-label="Профиль">
          {initial(name)}
        </Link>
      </div>
    </header>
  );
}

// BackBar tops every other screen: the way back to the catalogue, and an
// optional note on the right.
export function BackBar({
  note,
  label = "Назад",
  href = "/",
}: {
  note?: string;
  label?: string;
  href?: string;
}) {
  return (
    <nav className={styles.back}>
      <Link href={href} className={styles.backLink}>
        ← {label}
      </Link>
      {note && <span className={styles.note}>{note}</span>}
    </nav>
  );
}

const initial = (name: string): string => {
  return (Array.from(name.trim())[0] ?? "?").toUpperCase();
};
