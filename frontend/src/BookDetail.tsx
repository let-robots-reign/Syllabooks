import { useEffect, useState } from "react";
import clsx from "clsx";
import { useParams } from "react-router-dom";
import { api, ApiError, type CatalogBook } from "./api.ts";
import { bookLevel } from "./bookLevels.ts";
import { BookCover, LevelBars } from "./catalog/BookVisuals.tsx";
import { BottomAction } from "./catalog/BottomAction.tsx";
import { formatDueDate, pageWord } from "./catalog/presentation.ts";
import { BackBar } from "./Header.tsx";
import { ButtonLink } from "./ui/Button.tsx";
import styles from "./BookDetail.module.scss";

export function BookDetail() {
  const { id } = useParams();
  const [result, setResult] = useState<{
    id: string | undefined;
    book?: CatalogBook;
    error?: string;
    notFound?: boolean;
  }>();
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let current = true;
    api<CatalogBook>(`/books/${id}`).then(
      (book) => {
        if (current) setResult({ id, book });
      },
      (err: ApiError) => {
        if (!current) return;
        if (err.status === 404) setResult({ id, notFound: true });
        else setResult({ id, error: err.message });
      },
    );
    return () => {
      current = false;
    };
  }, [attempt, id]);

  const current = result?.id === id ? result : undefined;

  return (
    <>
      <BackBar label="Каталог" />
      {current?.notFound ? (
        <State title="Книга не найдена">
          Возможно, её сняли с полки. Вернись в каталог и выбери другую.
        </State>
      ) : current?.error ? (
        <State title="Книга не загрузилась">
          {current.error}
          <button
            type="button"
            onClick={() => {
              setResult(undefined);
              setAttempt((value) => value + 1);
            }}
          >
            Попробовать ещё раз
          </button>
        </State>
      ) : current?.book === undefined ? (
        <p className={styles.loading}>Загрузка книги…</p>
      ) : (
        <Book book={current.book} />
      )}
    </>
  );
}

function State({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section className={styles.state} role="status">
      <h1>{title}</h1>
      <div>{children}</div>
    </section>
  );
}

function Book({ book }: { book: CatalogBook }) {
  const level = bookLevel(book.level);
  const due = book.current_loan
    ? formatDueDate(book.current_loan.due_at)
    : null;

  let action;
  if (book.current_loan) {
    action = (
      <BottomAction
        note={`Книгу читает ${book.current_loan.borrower_name} — до ${due}`}
      />
    );
  } else if (!book.isbn) {
    action = (
      <BottomAction note="У этой книги нет ISBN. Чтобы взять её, обратись к учителю." />
    );
  } else {
    action = (
      <BottomAction note="Возьми книгу с полки и отсканируй штрих-код">
        <ButtonLink href="/scan">Сканировать книгу</ButtonLink>
      </BottomAction>
    );
  }

  return (
    <>
      <article className={styles.page}>
        <div className={styles.cover}>
          <BookCover book={book} hero />
        </div>

        <div className={clsx(styles.level, styles[`${book.level}Text`])}>
          <LevelBars level={book.level} />
          <span>
            {level.label} · уровень {level.number}
          </span>
        </div>
        <h1 className={styles.title}>{book.title}</h1>
        <p className={styles.author}>{book.author}</p>

        <dl className={styles.facts}>
          <div>
            <dt>Объём</dt>
            <dd>
              <strong>{book.page_count}</strong>
              <span>{pageWord(book.page_count)}</span>
            </dd>
          </div>
          <div>
            <dt>Наличие</dt>
            <dd
              className={clsx(
                styles.availability,
                book.current_loan ? styles.taken : styles.available,
              )}
            >
              {book.current_loan
                ? `Читает ${book.current_loan.borrower_name} · до ${due}`
                : "Свободна · на полке"}
            </dd>
          </div>
        </dl>

        {book.description && (
          <p className={styles.description}>{book.description}</p>
        )}
        <p className={clsx(styles.levelNote, styles[`${book.level}Border`])}>
          {level.detail}
        </p>
      </article>
      {action}
    </>
  );
}
