import { useEffect, useState } from "react";
import clsx from "clsx";
import {
  api,
  ApiError,
  type AdminLostBooksResponse,
  type AdminStats,
} from "../api.ts";
import { bookLevel } from "../bookLevels.ts";
import { formatDueDate } from "../catalog/presentation.ts";
import { Link } from "../ui/Link.tsx";
import { AdminShell } from "./AdminShell.tsx";
import styles from "./AdminLoans.module.scss";

export function AdminLost() {
  const [stats, setStats] = useState<AdminStats>();
  const [data, setData] = useState<AdminLostBooksResponse>();
  const [confirmation, setConfirmation] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let current = true;
    Promise.all([
      api<AdminStats>("/admin/stats"),
      api<AdminLostBooksResponse>("/admin/lost"),
    ]).then(
      ([nextStats, lost]) => {
        if (!current) return;
        setStats(nextStats);
        setData(lost);
      },
      (err: ApiError) => {
        if (current) setError(err.message);
      },
    );
    return () => {
      current = false;
    };
  }, []);

  const found = async (id: string) => {
    setBusy(id);
    setError(null);
    try {
      await api(`/admin/books/${id}/found`, { method: "POST" });
      setData((current) =>
        current
          ? { books: current.books.filter((item) => item.book.id !== id) }
          : current,
      );
      setStats((current) =>
        current
          ? { ...current, lost_books: Math.max(0, current.lost_books - 1) }
          : current,
      );
      setConfirmation(null);
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <AdminShell
      active="lost"
      stats={stats}
      actions={
        <Link href="/admin/books/new" className={styles.add}>
          Добавить книгу
        </Link>
      }
    >
      <p className={styles.lostIntro}>
        Утерянная книга уходит с полки, но остаётся в истории ученика. Если
        книга нашлась, «Вернулась» ставит её обратно в каталог.
      </p>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {data === undefined && !error ? (
        <p className={styles.state}>Загрузка утерянных книг…</p>
      ) : data?.books.length === 0 ? (
        <div className={styles.empty}>
          <h1>Утерянных книг нет</h1>
          <p>Все книги либо на полке, либо числятся за учениками.</p>
        </div>
      ) : (
        data && (
          <div className={styles.tableWrap}>
            <div className={styles.lostTable}>
              <div className={clsx(styles.lostRow, styles.tableHead)}>
                <span />
                <span>Книга</span>
                <span>Последний читатель</span>
                <span>Взято</span>
                <span>Отмечено</span>
                <span>Действие</span>
              </div>
              {data.books.map((item) => (
                <div className={styles.lostRow} key={item.book.id}>
                  <span
                    className={clsx(styles.spine, styles[item.book.level])}
                  />
                  <span className={styles.bookCell}>
                    <span className={styles.bookTitle}>{item.book.title}</span>
                    <span>
                      {bookLevel(item.book.level).shortLabel} ·{" "}
                      {item.book.page_count} с.
                      {item.book.isbn
                        ? ` · ISBN ${item.book.isbn}`
                        : " · без ISBN"}
                    </span>
                  </span>
                  <span>{item.last_borrower_name || "Нет истории выдач"}</span>
                  <span className={styles.muted}>
                    {item.last_taken_at
                      ? formatDueDate(item.last_taken_at)
                      : "—"}
                  </span>
                  <span className={styles.muted}>
                    {formatDueDate(item.lost_at)}
                  </span>
                  <span className={styles.actions}>
                    {confirmation === item.book.id ? (
                      <>
                        <button
                          type="button"
                          className={styles.accept}
                          disabled={busy === item.book.id}
                          onClick={() => found(item.book.id)}
                        >
                          {busy === item.book.id
                            ? "Сохраняем…"
                            : "Вернуть в каталог"}
                        </button>
                        <button
                          type="button"
                          disabled={busy === item.book.id}
                          onClick={() => setConfirmation(null)}
                        >
                          Отмена
                        </button>
                      </>
                    ) : (
                      <button
                        type="button"
                        className={styles.accept}
                        onClick={() => setConfirmation(item.book.id)}
                      >
                        Вернулась
                      </button>
                    )}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )
      )}
    </AdminShell>
  );
}
