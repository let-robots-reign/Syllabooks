import clsx from "clsx";
import type { Book, BookLevel, CatalogBook } from "../api.ts";
import { bookLevel } from "../bookLevels.ts";
import { formatDueDate } from "./presentation.ts";
import styles from "./BookVisuals.module.scss";

export function LevelBars({ level }: { level: BookLevel }) {
  const active = bookLevel(level).number;
  return (
    <span className={styles.bars} aria-hidden="true">
      {[1, 2, 3].map((bar) => (
        <span
          className={clsx(
            styles.bar,
            bar <= active ? styles[level] : styles.off,
          )}
          key={bar}
        />
      ))}
    </span>
  );
}

export function BookCover({
  book,
  hero = false,
  showLoan = false,
}: {
  book: Book | CatalogBook;
  hero?: boolean;
  showLoan?: boolean;
}) {
  return (
    <div className={clsx(styles.cover, hero && styles.hero)}>
      {book.cover_url ? (
        <img
          className={styles.image}
          src={book.cover_url}
          alt={`Обложка книги «${book.title}»`}
          loading={hero ? "eager" : "lazy"}
        />
      ) : (
        <div className={styles.fallback}>
          <span>без обложки</span>
          <strong>{book.title}</strong>
        </div>
      )}
      <span className={clsx(styles.spine, styles[book.level])} />
      {showLoan && "current_loan" in book && book.current_loan && (
        <>
          <span className={styles.veil} />
          <span className={styles.loan}>
            Читает {book.current_loan.borrower_name} — до{" "}
            {formatDueDate(book.current_loan.due_at)}
          </span>
        </>
      )}
    </div>
  );
}
