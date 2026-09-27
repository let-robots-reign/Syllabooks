import { useEffect, useLayoutEffect, useMemo, useState } from "react";
import clsx from "clsx";
import {
  useLocation,
  useNavigationType,
  useSearchParams,
} from "react-router-dom";
import type { BookLevel, CatalogBook, CatalogResponse, Me } from "./api.ts";
import { api, ApiError } from "./api.ts";
import { bookLevel, bookLevels } from "./bookLevels.ts";
import { BookRequest } from "./catalog/BookRequest.tsx";
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

const isBookLevel = (value: string | null): value is BookLevel =>
  bookLevels.some((level) => level.value === value);

// Where the catalogue was scrolled to, per history entry, so going back from
// a book lands on the same card rather than the top of the shelf.
const scrollPositions = new Map<string, number>();

export function Home({ me }: { me: Me }) {
  const location = useLocation();
  const navigationType = useNavigationType();
  const [searchParams, setSearchParams] = useSearchParams();
  const [catalog, setCatalog] = useState<CatalogResponse>();
  const [catalogLoadedAt, setCatalogLoadedAt] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);

  // Level filters live in the URL, so going back to the catalogue keeps them.
  // None selected shows every level; selected ones combine with OR.
  const selectedLevels = useMemo(
    () => searchParams.getAll("level").filter(isBookLevel),
    [searchParams],
  );

  useEffect(() => {
    if (selectedLevels.length !== searchParams.getAll("level").length) {
      setSearchParams({ level: selectedLevels }, { replace: true });
    }
  }, [searchParams, selectedLevels, setSearchParams]);

  useEffect(() => {
    const key = location.key;
    const remember = () => scrollPositions.set(key, window.scrollY);
    window.addEventListener("scroll", remember, { passive: true });
    return () => window.removeEventListener("scroll", remember);
  }, [location.key]);

  useLayoutEffect(() => {
    if (!catalog || navigationType !== "POP") return;
    const saved = scrollPositions.get(location.key);
    if (saved !== undefined) window.scrollTo(0, saved);
  }, [catalog, location.key, navigationType]);

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

  const activeCount = selectedLevels.length;
  const visibleBooks = useMemo(
    () =>
      catalog?.books.filter(
        (book) => activeCount === 0 || selectedLevels.includes(book.level),
      ) ?? [],
    [activeCount, catalog, selectedLevels],
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
    const next = selectedLevels.includes(level)
      ? selectedLevels.filter((selected) => selected !== level)
      : [...selectedLevels, level];
    setSearchParams({ level: next }, { replace: true });
  };

  const showAllLevels = () => setSearchParams({}, { replace: true });

  const onlyActiveLevel =
    activeCount === 1 ? bookLevel(selectedLevels[0]) : null;
  const finishedCount = catalog?.finished_count;
  const myLoan = catalog?.my_loan;

  return (
    <>
      <Header name={me.display_name} isAdmin={me.is_admin} />
      <section className={styles.page}>
        {finishedCount && finishedCount >= 3 && (
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
                  selectedLevels.includes(level.value) &&
                    styles[`${level.value}Active`],
                )}
                aria-pressed={selectedLevels.includes(level.value)}
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
              {activeCount > 0 && ` из ${catalog.books.length}`}
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
          <>
            <div className={clsx(styles.state, styles.emptyCatalogue)}>
              <div className={styles.emptyShelf} aria-hidden="true" />
              <h2>Полка пока пустая</h2>
              <p>
                Учитель добавляет книги по штрих-кодам. Зайди завтра — или
                напомни ему на уроке.
              </p>
            </div>
            <BookRequest />
          </>
        ) : visibleBooks.length === 0 ? (
          <div className={styles.state}>
            <h2>На выбранных уровнях книг пока нет</h2>
            <p>Выбери другой уровень и посмотри, что у нас есть.</p>
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
                  {activeCount > 0
                    ? "Пока можно посмотреть другие уровни."
                    : "Спроси, дочитана ли книга: возможно, её вернут раньше."}
                </p>
                {activeCount > 0 && (
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
            <BookRequest />
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
