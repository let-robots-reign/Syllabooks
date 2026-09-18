import { useEffect, useMemo, useState } from "react";
import clsx from "clsx";
import { useSearchParams } from "react-router-dom";
import type { BookLevel, CatalogBook, CatalogResponse, Me } from "./api.ts";
import { api, ApiError } from "./api.ts";
import { bookLevel, bookLevels } from "./bookLevels.ts";
import { BookCover, LevelBars } from "./catalog/BookVisuals.tsx";
import { BottomAction } from "./catalog/BottomAction.tsx";
import {
  booksReadWord,
  bookWord,
  formatDueDate,
  pageWord,
} from "./catalog/presentation.ts";
import { Header } from "./Header.tsx";
import { ButtonLink } from "./ui/Button.tsx";
import { Link } from "./ui/Link.tsx";
import styles from "./Home.module.scss";

const allLevels: Record<BookLevel, boolean> = {
  green: true,
  yellow: true,
  red: true,
};

const isBookLevel = (value: string | null): value is BookLevel =>
  bookLevels.some((level) => level.value === value);

export function Home({ me }: { me: Me }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const [catalog, setCatalog] = useState<CatalogResponse>();
  const [catalogLoadedAt, setCatalogLoadedAt] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [active, setActive] = useState<Record<BookLevel, boolean>>(() => {
    const requestedLevel = searchParams.get("level");
    if (isBookLevel(requestedLevel)) {
      return {
        green: requestedLevel === "green",
        yellow: requestedLevel === "yellow",
        red: requestedLevel === "red",
      };
    }
    return allLevels;
  });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    const requestedLevel = searchParams.get("level");
    if (requestedLevel && !isBookLevel(requestedLevel)) {
      setSearchParams({}, { replace: true });
    }
  }, [searchParams, setSearchParams]);

  useEffect(() => {
    let current = true;
    api<CatalogResponse>("/books").then(
      (result) => {
        if (current) {
          setCatalog(result);
          setCatalogLoadedAt(Date.now());
        }
      },
      (err: ApiError) => {
        if (current) setError(err.message);
      },
    );
    return () => {
      current = false;
    };
  }, [attempt]);

  const visibleBooks = useMemo(
    () => catalog?.books.filter((book) => active[book.level]) ?? [],
    [active, catalog],
  );

  const nearestUnavailable = useMemo(() => {
    const unavailable = visibleBooks.filter((book) => book.current_loan);
    if (
      unavailable.length !== visibleBooks.length ||
      unavailable.length === 0
    ) {
      return null;
    }
    return unavailable.toSorted(
      (left, right) =>
        new Date(left.current_loan!.due_at).getTime() -
        new Date(right.current_loan!.due_at).getTime(),
    )[0];
  }, [visibleBooks]);

  const toggleLevel = (level: BookLevel) => {
    if (searchParams.has("level")) setSearchParams({}, { replace: true });
    setActive((current) => {
      const next = { ...current, [level]: !current[level] };
      return Object.values(next).some(Boolean) ? next : { ...allLevels };
    });
  };

  const showAllLevels = () => {
    setActive(allLevels);
    if (searchParams.has("level")) setSearchParams({}, { replace: true });
  };

  const activeCount = Object.values(active).filter(Boolean).length;
  const onlyActiveLevel =
    activeCount === 1
      ? bookLevel(
          (Object.entries(active).find(([, enabled]) => enabled)?.[0] ??
            "green") as BookLevel,
        )
      : null;
  const finishedCount = catalog?.finished_count;
  const myLoan = catalog?.my_loan;

  return (
    <>
      <Header name={me.display_name} isAdmin={me.is_admin} />
      <section className={styles.page}>
        {!!finishedCount && (
          <div className={styles.counter}>
            <div className={styles.eyebrow}>Общий счёт</div>
            <div className={styles.counterTitle}>
              <strong>{finishedCount}</strong>
              <span>
                {booksReadWord(finishedCount)} прочитал
                <br />
                наш книжный клуб
              </span>
            </div>

            <div className={styles.counterShelf} aria-hidden="true">
              {Array.from({ length: finishedCount }, (_, index) => (
                <span key={index} />
              ))}
            </div>
          </div>
        )}

        <div className={styles.filters}>
          <div className={styles.eyebrow}>Уровень</div>
          <div className={styles.filterGrid} aria-label="Фильтр по уровню">
            {bookLevels.map((level) => (
              <button
                type="button"
                className={clsx(
                  styles.filter,
                  styles[`${level.value}Text`],
                  active[level.value] && styles[`${level.value}Active`],
                )}
                aria-pressed={active[level.value]}
                onClick={() => toggleLevel(level.value)}
                key={level.value}
              >
                <LevelBars level={level.value} />
                <span>{level.label}</span>
              </button>
            ))}
          </div>
        </div>

        <div className={styles.shelfHeading}>
          <h1>На полке</h1>
          {catalog && (
            <span>
              {visibleBooks.length} {bookWord(visibleBooks.length)}
              {activeCount < 3 && ` из ${catalog.books.length}`}
            </span>
          )}
        </div>

        {error ? (
          <div className={styles.state} role="alert">
            <h2>Каталог не загрузился</h2>
            <p>{error}</p>
            <button
              type="button"
              onClick={() => {
                setCatalog(undefined);
                setError(null);
                setAttempt((value) => value + 1);
              }}
            >
              Попробовать ещё раз
            </button>
          </div>
        ) : catalog === undefined ? (
          <p className={styles.loading}>Загрузка каталога…</p>
        ) : catalog.books.length === 0 ? (
          <div className={clsx(styles.state, styles.emptyCatalogue)}>
            <div className={styles.emptyShelf} aria-hidden="true" />
            <h2>Полка пока пустая</h2>
            <p>
              Учитель добавляет книги по штрих-кодам. Зайди завтра — или напомни
              ему на уроке.
            </p>
          </div>
        ) : visibleBooks.length === 0 ? (
          <div className={styles.state}>
            <h2>На выбранных уровнях книг пока нет</h2>
            <p>Выбери другой уровень и посмотри, что есть на полке.</p>
            <button type="button" onClick={showAllLevels}>
              Показать все уровни
            </button>
          </div>
        ) : (
          <>
            {nearestUnavailable?.current_loan && (
              <div className={clsx(styles.state, styles.unavailableState)}>
                <h2>
                  {onlyActiveLevel
                    ? `Все книги уровня «${onlyActiveLevel.label}» сейчас на руках`
                    : "Все книги на выбранных уровнях сейчас на руках"}
                </h2>
                <p>
                  Ближайшая — {nearestUnavailable.title} у{" "}
                  {nearestUnavailable.current_loan.borrower_name}:{" "}
                  {new Date(nearestUnavailable.current_loan.due_at).getTime() <
                  catalogLoadedAt
                    ? "срок уже прошёл"
                    : `до ${formatDueDate(nearestUnavailable.current_loan.due_at)}`}
                  .{" "}
                  {activeCount < 3
                    ? "Пока можно посмотреть другие уровни."
                    : "Спроси, дочитана ли книга: возможно, её вернут раньше."}
                </p>
                {activeCount < 3 && (
                  <button type="button" onClick={showAllLevels}>
                    Показать все уровни
                  </button>
                )}
              </div>
            )}
            <div className={styles.books}>
              {visibleBooks.map((book) => (
                <BookCard book={book} key={book.id} />
              ))}
            </div>
          </>
        )}
      </section>

      <BottomAction>
        <ButtonLink href={myLoan ? "/return" : "/scan"}>
          {myLoan ? `Вернуть «${myLoan.book.title}»` : "Сканировать книгу"}
        </ButtonLink>
      </BottomAction>
    </>
  );
}

function BookCard({ book }: { book: CatalogBook }) {
  const level = bookLevel(book.level);
  const availability = book.current_loan
    ? `Занята · до ${formatDueDate(book.current_loan.due_at)}`
    : "Свободна";

  return (
    <Link
      href={`/books/${book.id}`}
      className={styles.book}
      aria-label={`${book.title}, ${level.label}, ${availability}`}
    >
      <BookCover book={book} showLoan />
      <div>
        <h2>{book.title}</h2>
        <p className={styles.author}>{book.author}</p>
      </div>
      <div className={clsx(styles.cardLevel, styles[`${book.level}Text`])}>
        <LevelBars level={book.level} />
        <span>{level.label}</span>
      </div>
      <div className={styles.pages}>
        <strong>{book.page_count}</strong>
        <span>{pageWord(book.page_count)}</span>
      </div>
      <p
        className={clsx(
          styles.availability,
          book.current_loan ? styles.taken : styles.available,
        )}
      >
        {availability}
      </p>
    </Link>
  );
}
