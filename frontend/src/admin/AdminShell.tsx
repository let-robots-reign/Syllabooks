import type { ReactNode } from "react";
import clsx from "clsx";
import type { AdminStats } from "../api.ts";
import { Link } from "../ui/Link.tsx";
import { Wordmark } from "../ui/Wordmark.tsx";
import styles from "./AdminShell.module.scss";

type AdminSection = "loans" | "books" | "users" | "lost";

export function AdminShell({
  active,
  stats,
  actions,
  children,
}: {
  active: AdminSection;
  stats?: AdminStats;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className={styles.page}>
      <header className={styles.header}>
        <Link href="/" className={styles.home} aria-label="Открыть каталог">
          <Wordmark size={24} />
        </Link>
        <div className={styles.headerActions}>
          <details className={styles.exportMenu}>
            <summary>Выгрузить CSV</summary>
            <div className={styles.exportOptions}>
              <Link
                href="/api/admin/exports/books.csv"
                download
                aria-label="Скачать CSV: книги"
              >
                Книги
              </Link>
              <Link
                href="/api/admin/exports/users.csv"
                download
                aria-label="Скачать CSV: пользователи"
              >
                Пользователи
              </Link>
              <Link
                href="/api/admin/exports/loans.csv"
                download
                aria-label="Скачать CSV: выдачи"
              >
                Выдачи
              </Link>
            </div>
          </details>
          {actions}
        </div>
      </header>
      <nav className={styles.nav} aria-label="Разделы кабинета учителя">
        <AdminLink active={active === "loans"} href="/admin/loans">
          Выдачи · {stats?.open_loans ?? "…"}
        </AdminLink>
        <AdminLink active={active === "books"} href="/admin/books">
          Каталог · {stats?.books ?? "…"}
        </AdminLink>
        <AdminLink active={active === "users"} href="/admin/users">
          Ученики · {stats?.users ?? "…"}
        </AdminLink>
        <AdminLink active={active === "lost"} href="/admin/lost">
          Утеряно · {stats?.lost_books ?? "…"}
        </AdminLink>
      </nav>
      {children}
    </section>
  );
}

function AdminLink({
  active,
  href,
  children,
}: {
  active: boolean;
  href: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className={clsx(styles.navLink, active && styles.active)}
      aria-current={active ? "page" : undefined}
    >
      {children}
    </Link>
  );
}
