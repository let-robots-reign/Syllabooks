import { useEffect, useState } from "react";
import clsx from "clsx";
import {
  api,
  ApiError,
  type AdminBookRequest,
  type AdminBookRequestsResponse,
  type AdminStats,
} from "../api.ts";
import { formatDueDate } from "../catalog/presentation.ts";
import { Link } from "../ui/Link.tsx";
import { AdminShell } from "./AdminShell.tsx";
import loanStyles from "./AdminLoans.module.scss";
import styles from "./AdminBookRequests.module.scss";

export function AdminBookRequests() {
  const [stats, setStats] = useState<AdminStats>();
  const [requests, setRequests] = useState<AdminBookRequest[]>();
  const [confirmation, setConfirmation] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let current = true;
    Promise.all([
      api<AdminStats>("/admin/stats"),
      api<AdminBookRequestsResponse>("/admin/book-requests"),
    ]).then(
      ([nextStats, data]) => {
        if (!current) return;
        setStats(nextStats);
        setRequests(data.requests);
      },
      (err: ApiError) => {
        if (current) setError(err.message);
      },
    );
    return () => {
      current = false;
    };
  }, []);

  const countOpen = (list: AdminBookRequest[]) =>
    list.filter((request) => !request.purchased_at).length;

  const update = (next: AdminBookRequest[]) => {
    setRequests(next);
    setStats((current) =>
      current ? { ...current, open_book_requests: countOpen(next) } : current,
    );
  };

  const togglePurchased = async (request: AdminBookRequest) => {
    if (!requests) return;
    const purchased = !request.purchased_at;
    setBusy(request.id);
    setError(null);
    try {
      await api(`/admin/book-requests/${request.id}/purchased`, {
        method: "PUT",
        body: { purchased },
      });
      // Keep the row where it is until the next visit, so a mis-click can be
      // undone in place.
      update(
        requests.map((item) =>
          item.id === request.id
            ? {
                ...item,
                purchased_at: purchased ? new Date().toISOString() : null,
              }
            : item,
        ),
      );
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(null);
    }
  };

  const remove = async (id: string) => {
    if (!requests) return;
    setBusy(id);
    setError(null);
    try {
      await api(`/admin/book-requests/${id}`, { method: "DELETE" });
      update(requests.filter((item) => item.id !== id));
      setConfirmation(null);
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <AdminShell
      active="requests"
      stats={stats}
      actions={
        <Link href="/admin/books/new" className={loanStyles.add}>
          Добавить книгу
        </Link>
      }
    >
      <p className={loanStyles.lostIntro}>
        Книги, которые ученики хотели бы прочитать. Отметь галочкой купленные
        книги.
      </p>

      {error && (
        <p className={loanStyles.error} role="alert">
          {error}
        </p>
      )}

      {requests === undefined && !error ? (
        <p className={loanStyles.state}>Загрузка предложений…</p>
      ) : requests?.length === 0 ? (
        <div className={loanStyles.empty}>
          <h1>Предложений пока нет</h1>
          <p>Ученики могут предложить книгу внизу каталога.</p>
        </div>
      ) : (
        requests && (
          <div className={loanStyles.tableWrap}>
            <div className={styles.table}>
              <div className={clsx(styles.row, loanStyles.tableHead)}>
                <span />
                <span>Книга</span>
                <span>Кто предложил</span>
                <span>Дата</span>
                <span>Действие</span>
              </div>
              {requests.map((request) => {
                const purchased = Boolean(request.purchased_at);
                return (
                  <div
                    className={clsx(styles.row, purchased && styles.purchased)}
                    key={request.id}
                  >
                    <button
                      type="button"
                      className={clsx(styles.tick, purchased && styles.ticked)}
                      aria-pressed={purchased}
                      aria-label={`Куплено: ${request.title}`}
                      title={purchased ? "Снять отметку" : "Отметить купленной"}
                      disabled={busy === request.id}
                      onClick={() => togglePurchased(request)}
                    >
                      <svg viewBox="0 0 16 16" aria-hidden="true">
                        <path d="M4 8.4l2.6 2.6L12 5.4" />
                      </svg>
                    </button>
                    <span className={styles.title}>{request.title}</span>
                    <span>{request.student.display_name}</span>
                    <span className={loanStyles.muted}>
                      {formatDueDate(request.created_at)}
                    </span>
                    <span className={loanStyles.actions}>
                      {confirmation === request.id ? (
                        <>
                          <button
                            type="button"
                            className={loanStyles.danger}
                            disabled={busy === request.id}
                            onClick={() => remove(request.id)}
                          >
                            {busy === request.id ? "Удаляем…" : "Да, удалить"}
                          </button>
                          <button
                            type="button"
                            disabled={busy === request.id}
                            onClick={() => setConfirmation(null)}
                          >
                            Отмена
                          </button>
                        </>
                      ) : (
                        <button
                          type="button"
                          onClick={() => setConfirmation(request.id)}
                        >
                          Удалить
                        </button>
                      )}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        )
      )}
    </AdminShell>
  );
}
