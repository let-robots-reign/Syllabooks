import { useEffect, useMemo, useState } from "react";
import clsx from "clsx";
import {
  api,
  ApiError,
  type AdminStats,
  type Book,
  type BookLevel,
} from "../api.ts";
import { Link } from "../ui/Link.tsx";
import { bookLevels, levelLabel } from "../bookLevels.ts";
import { AdminShell } from "./AdminShell.tsx";
import styles from "./AdminBooks.module.scss";

type Filter = "all" | BookLevel | "missing-cover";

export function AdminBooks() {
  const [books, setBooks] = useState<Book[]>();
  const [stats, setStats] = useState<AdminStats>();
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [pendingDelete, setPendingDelete] = useState<string | null>(null);
  const [draggedID, setDraggedID] = useState<string | null>(null);
  const [savingOrder, setSavingOrder] = useState(false);

  useEffect(() => {
    let current = true;
    Promise.all([
      api<Book[]>("/admin/books"),
      api<AdminStats>("/admin/stats"),
    ]).then(
      ([result, nextStats]) => {
        if (!current) return;
        setBooks(result);
        setStats(nextStats);
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

  const chooseFilter = (value: Filter) => {
    setFilter(value);
    setPendingDelete(null);
  };

  const dropBook = async (targetID: string) => {
    if (!books || !draggedID || draggedID === targetID || savingOrder) return;
    const previous = books;
    const next = reorderFilteredBooks(
      books,
      filtered.map((book) => book.id),
      draggedID,
      targetID,
    );
    setBooks(next);
    setDraggedID(null);
    setSavingOrder(true);
    setError(null);
    try {
      await api("/admin/books/order", {
        method: "PUT",
        body: { ids: next.map((book) => book.id) },
      });
    } catch (err) {
      setBooks(previous);
      setError((err as ApiError).message);
    } finally {
      setSavingOrder(false);
    }
  };

  const remove = async (book: Book) => {
    setError(null);
    try {
      await api(`/admin/books/${book.id}`, { method: "DELETE" });
      setBooks((current) => current?.filter((item) => item.id !== book.id));
      setStats((current) =>
        current
          ? { ...current, books: Math.max(0, current.books - 1) }
          : current,
      );
      setPendingDelete(null);
    } catch (err) {
      setError((err as ApiError).message);
    }
  };

  return (
    <AdminShell
      active="books"
      stats={stats}
      actions={
        <>
          <label className={styles.search}>
            <span className={styles.visuallyHidden}>Поиск по названию</span>
            <input
              type="search"
              value={query}
              placeholder="Поиск по названию"
              onChange={(event) => {
                setQuery(event.target.value);
              }}
            />
          </label>
          <Link href="/admin/books/new" className={styles.add}>
            Добавить книгу
          </Link>
        </>
      }
    >
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
            {filtered.map((book) => (
              <div
                className={clsx(
                  styles.row,
                  !book.cover_url && styles.noCover,
                  draggedID === book.id && styles.dragging,
                )}
                key={book.id}
                onDragOver={(event) => event.preventDefault()}
                onDrop={(event) => {
                  event.preventDefault();
                  void dropBook(book.id);
                }}
              >
                <div className={styles.visualCell}>
                  <button
                    type="button"
                    className={styles.dragHandle}
                    draggable={!savingOrder}
                    aria-label={`Изменить порядок книги «${book.title}»`}
                    title="Перетащить"
                    onDragStart={(event) => {
                      event.dataTransfer.effectAllowed = "move";
                      event.dataTransfer.setData("text/plain", book.id);
                      setDraggedID(book.id);
                    }}
                    onDragEnd={() => setDraggedID(null)}
                  >
                    ⋮⋮
                  </button>
                  <span className={clsx(styles.spine, styles[book.level])} />
                  {book.cover_url && (
                    <img
                      className={styles.cover}
                      src={book.cover_url}
                      alt=""
                    />
                  )}
                </div>
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
              {filtered.length} из {books.length} · порядок можно менять
              перетаскиванием
            </span>
            {savingOrder && <span>Сохраняем порядок…</span>}
          </footer>
        </>
      )}
    </AdminShell>
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

function reorderFilteredBooks(
  books: Book[],
  filteredIDs: string[],
  draggedID: string,
  targetID: string,
) {
  const reorderedIDs = [...filteredIDs];
  const from = reorderedIDs.indexOf(draggedID);
  const to = reorderedIDs.indexOf(targetID);
  if (from < 0 || to < 0) return books;
  reorderedIDs.splice(from, 1);
  reorderedIDs.splice(to, 0, draggedID);

  const byID = new Map(books.map((book) => [book.id, book]));
  const visible = new Set(filteredIDs);
  let index = 0;
  return books.map((book) =>
    visible.has(book.id) ? byID.get(reorderedIDs[index++])! : book,
  );
}
