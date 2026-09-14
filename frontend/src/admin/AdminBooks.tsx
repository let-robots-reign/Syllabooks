import { useEffect, useMemo, useState } from "react";
import clsx from "clsx";
import { api, ApiError, type Book, type BookLevel } from "../api.ts";
import { Link } from "../ui/Link.tsx";
import { Wordmark } from "../ui/Wordmark.tsx";
import { bookLevels, levelLabel } from "./bookLevels.ts";
import styles from "./AdminBooks.module.scss";

type Filter = "all" | BookLevel | "missing-cover";

export function AdminBooks() {
  const [books, setBooks] = useState<Book[]>();
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [showAll, setShowAll] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<string | null>(null);

  useEffect(() => {
    let current = true;
    api<Book[]>("/admin/books").then(
      (result) => {
        if (current) setBooks(result);
      },
      (err: ApiError) => {
        if (current) setError(err.message);
      },
    );
    return () => {
      current = false;
    };
  }, []);

  const counts = useMemo(() => {
    const count: Record<Filter, number> = {
      all: books?.length ?? 0,
      green: 0,
      yellow: 0,
      red: 0,
      "missing-cover": 0,
    };
    for (const book of books ?? []) {
      count[book.level] += 1;
      if (!book.cover_url) count["missing-cover"] += 1;
    }
    return count;
  }, [books]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase("ru");
    return (books ?? []).filter((book) => {
      if (filter === "missing-cover" && book.cover_url) return false;
      if (
        filter !== "all" &&
        filter !== "missing-cover" &&
        book.level !== filter
      ) {
        return false;
      }
      return !needle || book.title.toLocaleLowerCase("ru").includes(needle);
    });
  }, [books, filter, query]);

  const visible = showAll ? filtered : filtered.slice(0, 8);

  const chooseFilter = (value: Filter) => {
    setFilter(value);
    setShowAll(false);
    setPendingDelete(null);
  };

  const remove = async (book: Book) => {
    setError(null);
    try {
      await api(`/admin/books/${book.id}`, { method: "DELETE" });
      setBooks((current) => current?.filter((item) => item.id !== book.id));
      setPendingDelete(null);
    } catch (err) {
      setError((err as ApiError).message);
    }
  };

  return (
    <section className={styles.page}>
      <header className={styles.header}>
        <Wordmark size={24} />
        <div className={styles.headerActions}>
          <label className={styles.search}>
            <span className={styles.visuallyHidden}>Поиск по названию</span>
            <input
              type="search"
              value={query}
              placeholder="Поиск по названию"
              onChange={(event) => {
                setQuery(event.target.value);
                setShowAll(false);
              }}
            />
          </label>
          <Link href="/admin/books/new" className={styles.add}>
            Добавить книгу
          </Link>
        </div>
      </header>

      <div className={styles.rule} />

      <div className={styles.filters} aria-label="Фильтр каталога">
        <FilterButton
          active={filter === "all"}
          onClick={() => chooseFilter("all")}
        >
          Все · {counts.all}
        </FilterButton>
        {bookLevels.map((level) => (
          <FilterButton
            key={level.value}
            level={level.value}
            active={filter === level.value}
            onClick={() => chooseFilter(level.value)}
          >
            <span className={clsx(styles.filterSpine, styles[level.value])} />
            {level.shortLabel} · {counts[level.value]}
          </FilterButton>
        ))}
        <FilterButton
          active={filter === "missing-cover"}
          onClick={() => chooseFilter("missing-cover")}
        >
          Без обложки · {counts["missing-cover"]}
        </FilterButton>
      </div>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {books === undefined ? (
        <p className={styles.state}>Загрузка каталога…</p>
      ) : filtered.length === 0 ? (
        <div className={styles.empty}>
          <h1>
            {books.length === 0 ? "Каталог пока пуст" : "Ничего не найдено"}
          </h1>
          <p>
            {books.length === 0
              ? "Добавь первую книгу — можно начать с ISBN."
              : "Измени запрос или выбери другой фильтр."}
          </p>
        </div>
      ) : (
        <>
          <div className={styles.table}>
            <div className={clsx(styles.row, styles.tableHead)}>
              <span />
              <span>Книга</span>
              <span>Автор</span>
              <span>Уровень</span>
              <span>Стр.</span>
              <span>Действия</span>
            </div>
            {visible.map((book) => (
              <div
                className={clsx(styles.row, !book.cover_url && styles.noCover)}
                key={book.id}
              >
                <span className={clsx(styles.spine, styles[book.level])} />
                <div className={styles.bookCell}>
                  <span className={styles.bookTitle}>{book.title}</span>
                  <span className={clsx(!book.cover_url && styles.missing)}>
                    {book.isbn ? `ISBN ${formatISBN(book.isbn)}` : "без ISBN"}
                    {!book.cover_url && " · нет обложки"}
                  </span>
                </div>
                <span>{book.author}</span>
                <span
                  className={clsx(styles.level, styles[`${book.level}Text`])}
                >
                  {levelLabel(book.level)}
                </span>
                <span>{book.page_count}</span>
                <div className={styles.actions}>
                  {pendingDelete === book.id ? (
                    <>
                      <button
                        className={styles.danger}
                        onClick={() => remove(book)}
                      >
                        Удалить навсегда
                      </button>
                      <button onClick={() => setPendingDelete(null)}>
                        Отмена
                      </button>
                    </>
                  ) : (
                    <>
                      <Link href={`/admin/books/${book.id}`}>Открыть</Link>
                      <button onClick={() => setPendingDelete(book.id)}>
                        Снять
                      </button>
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>
          <footer className={styles.footer}>
            <span>
              Показаны {visible.length} из {filtered.length} · сортировка по
              названию
            </span>
            {!showAll && filtered.length > 8 && (
              <button onClick={() => setShowAll(true)}>
                Показать все {filtered.length} →
              </button>
            )}
          </footer>
        </>
      )}
    </section>
  );
}

function FilterButton({
  active,
  level,
  children,
  onClick,
}: {
  active: boolean;
  level?: BookLevel;
  children: React.ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={clsx(
        styles.filter,
        active && styles.active,
        level && styles[`${level}Text`],
      )}
      aria-pressed={active}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

const formatISBN = (isbn: string) => {
  return `${isbn.slice(0, 3)}-${isbn.slice(3, 4)}-${isbn.slice(4, 7)}-${isbn.slice(7, 12)}-${isbn.slice(12)}`;
};
