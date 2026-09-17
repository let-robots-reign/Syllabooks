import { useEffect, useMemo, useState } from "react";
import clsx from "clsx";
import {
  api,
  ApiError,
  type AdminLoansResponse,
  type AdminOpenLoan,
  type AdminStats,
} from "../api.ts";
import { bookLevel } from "../bookLevels.ts";
import { formatDueDate } from "../catalog/presentation.ts";
import { Link } from "../ui/Link.tsx";
import { AdminShell } from "./AdminShell.tsx";
import styles from "./AdminLoans.module.scss";

type LoanFilter = "all" | "overdue" | "week";
type Confirmation = { id: string; action: "return" | "lost" } | null;

const day = 24 * 60 * 60 * 1000;

export function AdminLoans() {
  const [stats, setStats] = useState<AdminStats>();
  const [data, setData] = useState<AdminLoansResponse>();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<LoanFilter>("all");
  const [confirmation, setConfirmation] = useState<Confirmation>(null);
  const [busy, setBusy] = useState<Set<string>>(() => new Set());
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let current = true;
    Promise.all([
      api<AdminStats>("/admin/stats"),
      api<AdminLoansResponse>("/admin/loans"),
    ]).then(
      ([nextStats, loans]) => {
        if (!current) return;
        setStats(nextStats);
        setData(loans);
      },
      (err: ApiError) => {
        if (current) setError(err.message);
      },
    );
    return () => {
      current = false;
    };
  }, []);

  const asOf = data ? new Date(data.as_of).getTime() : 0;
  const counts = useMemo(() => {
    const loans = data?.open_loans ?? [];
    return {
      all: loans.length,
      overdue: loans.filter((loan) => isOverdue(loan, asOf)).length,
      week: loans.filter((loan) => isDueThisWeek(loan, asOf)).length,
    };
  }, [asOf, data]);

  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase("ru");
    return (data?.open_loans ?? []).filter((loan) => {
      if (
        needle &&
        !loan.student.display_name.toLocaleLowerCase("ru").includes(needle)
      ) {
        return false;
      }
      if (filter === "overdue") return isOverdue(loan, asOf);
      if (filter === "week") return isDueThisWeek(loan, asOf);
      return true;
    });
  }, [asOf, data, filter, query]);

  const extend = async (loan: AdminOpenLoan) => {
    const key = `extend:${loan.id}`;
    setBusy((current) => new Set(current).add(key));
    setError(null);
    try {
      const result = await api<{ due_at: string }>(
        `/admin/loans/${loan.id}/extend`,
        { method: "POST" },
      );
      setData((current) =>
        current
          ? {
              ...current,
              open_loans: current.open_loans.map((item) =>
                item.id === loan.id ? { ...item, due_at: result.due_at } : item,
              ),
            }
          : current,
      );
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy((current) => without(current, key));
    }
  };

  const close = async (loan: AdminOpenLoan, action: "return" | "lost") => {
    const key = `${action}:${loan.id}`;
    setBusy((current) => new Set(current).add(key));
    setError(null);
    try {
      await api(`/admin/loans/${loan.id}/${action}`, { method: "POST" });
      setData((current) =>
        current
          ? {
              ...current,
              open_loans: current.open_loans.filter(
                (item) => item.id !== loan.id,
              ),
            }
          : current,
      );
      setStats((current) =>
        current
          ? {
              ...current,
              open_loans: Math.max(0, current.open_loans - 1),
              lost_books: current.lost_books + (action === "lost" ? 1 : 0),
            }
          : current,
      );
      setConfirmation((current) =>
        current?.id === loan.id && current.action === action ? null : current,
      );
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy((current) => without(current, key));
    }
  };

  const review = async (id: string) => {
    const key = `review:${id}`;
    setBusy((current) => new Set(current).add(key));
    setError(null);
    try {
      await api(`/admin/loans/${id}/review`, { method: "POST" });
      setData((current) =>
        current
          ? {
              ...current,
              review_queue: current.review_queue.filter(
                (item) => item.id !== id,
              ),
            }
          : current,
      );
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy((current) => without(current, key));
    }
  };

  return (
    <AdminShell
      active="loans"
      stats={stats}
      actions={
        <>
          <label className={styles.search}>
            <span className={styles.visuallyHidden}>Поиск по ученику</span>
            <input
              type="search"
              value={query}
              placeholder="Поиск по ученику"
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          <Link href="/admin/books/new" className={styles.add}>
            Добавить книгу
          </Link>
        </>
      }
    >
      <div className={styles.filters} aria-label="Фильтр выдач">
        <FilterButton
          active={filter === "overdue"}
          danger
          onClick={() => setFilter("overdue")}
        >
          Просрочено · {counts.overdue}
        </FilterButton>
        <FilterButton
          active={filter === "all"}
          onClick={() => setFilter("all")}
        >
          Все открытые · {counts.all}
        </FilterButton>
        <FilterButton
          active={filter === "week"}
          onClick={() => setFilter("week")}
        >
          Возврат за 7 дней · {counts.week}
        </FilterButton>
      </div>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {data === undefined && !error ? (
        <p className={styles.state}>Загрузка выдач…</p>
      ) : data && visible.length === 0 ? (
        <div className={styles.empty}>
          <h1>
            {data.open_loans.length === 0
              ? "Открытых выдач нет"
              : "По этому фильтру ничего нет"}
          </h1>
          <p>
            {data.open_loans.length === 0
              ? "Открытых выдач сейчас нет."
              : "Измени поиск или выбери другой период."}
          </p>
        </div>
      ) : (
        data && (
          <>
            <div className={styles.tableWrap}>
              <div className={styles.loanTable}>
                <div className={clsx(styles.loanRow, styles.tableHead)}>
                  <span />
                  <span>Книга</span>
                  <span>Ученик</span>
                  <span>Взято</span>
                  <span>Вернуть</span>
                  <span>Статус</span>
                  <span>Действия</span>
                </div>
                {visible.map((loan) => {
                  const overdue = isOverdue(loan, asOf);
                  return (
                    <div
                      className={clsx(
                        styles.loanRow,
                        overdue && styles.overdueRow,
                      )}
                      key={loan.id}
                    >
                      <span
                        className={clsx(styles.spine, styles[loan.book.level])}
                      />
                      <BookCell loan={loan} />
                      <span>{loan.student.display_name}</span>
                      <span className={styles.muted}>
                        {formatDueDate(loan.taken_at)}
                      </span>
                      <span className={styles.due}>
                        {formatDueDate(loan.due_at)}
                      </span>
                      <span
                        className={clsx(
                          styles.status,
                          overdue && styles.overdue,
                        )}
                      >
                        {loanStatus(loan, asOf)}
                      </span>
                      <LoanActions
                        loan={loan}
                        confirmation={confirmation}
                        busy={busy}
                        onConfirm={setConfirmation}
                        onCancel={() => setConfirmation(null)}
                        onExtend={() => extend(loan)}
                        onClose={(action) => close(loan, action)}
                      />
                    </div>
                  );
                })}
              </div>
            </div>
            <p className={styles.footer}>
              {visible.length} из {data.open_loans.length} открытых выдач.
              «Принять» закрывает выдачу от имени ученика — без сканирования.
            </p>
          </>
        )
      )}

      {data && data.review_queue.length > 0 && (
        <section className={styles.reviewSection}>
          <div className={styles.reviewHeading}>
            <h2>Требуют сверки · {data.review_queue.length}</h2>
            <p>Возвраты, где полка или книга не были отсканированы.</p>
          </div>
          <div className={styles.tableWrap}>
            <div className={styles.reviewTable}>
              <div className={clsx(styles.reviewRow, styles.tableHead)}>
                <span />
                <span>Книга</span>
                <span>Ученик</span>
                <span>Возвращена</span>
                <span>Предупреждение</span>
                <span>Действие</span>
              </div>
              {data.review_queue.map((loan) => (
                <div className={styles.reviewRow} key={loan.id}>
                  <span
                    className={clsx(styles.spine, styles[loan.book.level])}
                  />
                  <BookCell loan={loan} />
                  <span>{loan.student.display_name}</span>
                  <span className={styles.muted}>
                    {formatDueDate(loan.returned_at)}
                  </span>
                  <span className={styles.warning}>
                    {scanWarning(loan.shelf_scan_ok, loan.book_scan_ok)}
                  </span>
                  <button
                    type="button"
                    className={styles.accept}
                    disabled={busy.has(`review:${loan.id}`)}
                    onClick={() => review(loan.id)}
                  >
                    {busy.has(`review:${loan.id}`) ? "Сохраняем…" : "Сверено"}
                  </button>
                </div>
              ))}
            </div>
          </div>
        </section>
      )}
    </AdminShell>
  );
}

function FilterButton({
  active,
  danger = false,
  onClick,
  children,
}: {
  active: boolean;
  danger?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      className={clsx(
        styles.filter,
        active && styles.activeFilter,
        danger && styles.dangerFilter,
      )}
      aria-pressed={active}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

function BookCell({ loan }: { loan: AdminOpenLoan }) {
  return (
    <span className={styles.bookCell}>
      <span className={styles.bookTitle}>{loan.book.title}</span>
      <span>
        {bookLevel(loan.book.level).shortLabel} · {loan.book.page_count} с.
        {loan.book.isbn
          ? ` · ISBN ${formatISBN(loan.book.isbn)}`
          : " · без ISBN"}
      </span>
    </span>
  );
}

function LoanActions({
  loan,
  confirmation,
  busy,
  onConfirm,
  onCancel,
  onExtend,
  onClose,
}: {
  loan: AdminOpenLoan;
  confirmation: Confirmation;
  busy: Set<string>;
  onConfirm: (confirmation: Confirmation) => void;
  onCancel: () => void;
  onExtend: () => void;
  onClose: (action: "return" | "lost") => void;
}) {
  const rowBusy = [...busy].some((key) => key.endsWith(`:${loan.id}`));
  if (confirmation?.id === loan.id) {
    const lost = confirmation.action === "lost";
    const key = `${confirmation.action}:${loan.id}`;
    return (
      <span className={styles.actions}>
        <button
          type="button"
          className={lost ? styles.danger : styles.accept}
          disabled={busy.has(key)}
          onClick={() => onClose(confirmation.action)}
        >
          {busy.has(key)
            ? "Сохраняем…"
            : lost
              ? "Отметить утерянной"
              : "Принять книгу"}
        </button>
        <button type="button" disabled={busy.has(key)} onClick={onCancel}>
          Отмена
        </button>
      </span>
    );
  }
  return (
    <span className={styles.actions}>
      <button
        type="button"
        className={styles.accept}
        disabled={rowBusy}
        onClick={() => onConfirm({ id: loan.id, action: "return" })}
      >
        Принять
      </button>
      <button type="button" disabled={rowBusy} onClick={onExtend}>
        {busy.has(`extend:${loan.id}`) ? "Продлеваем…" : "+7 дней"}
      </button>
      <button
        type="button"
        className={styles.danger}
        disabled={rowBusy}
        onClick={() => onConfirm({ id: loan.id, action: "lost" })}
      >
        Утеряна
      </button>
    </span>
  );
}

function without(values: Set<string>, value: string) {
  const next = new Set(values);
  next.delete(value);
  return next;
}

const isOverdue = (loan: AdminOpenLoan, asOf: number) =>
  new Date(loan.due_at).getTime() < asOf;

const isDueThisWeek = (loan: AdminOpenLoan, asOf: number) => {
  const due = new Date(loan.due_at).getTime();
  return due >= asOf && due < asOf + 7 * day;
};

const loanStatus = (loan: AdminOpenLoan, asOf: number) => {
  const delta = new Date(loan.due_at).getTime() - asOf;
  if (delta < 0) {
    const days = Math.max(1, Math.ceil(Math.abs(delta) / day));
    return `Просрочено ${days} ${dayWord(days)}`;
  }
  const days = Math.max(1, Math.ceil(delta / day));
  return `Осталось ${days} ${dayWord(days)}`;
};

const dayWord = (value: number) => {
  const mod100 = value % 100;
  const mod10 = value % 10;
  if (mod100 >= 11 && mod100 <= 14) return "дн.";
  if (mod10 === 1) return "день";
  if (mod10 >= 2 && mod10 <= 4) return "дня";
  return "дн.";
};

const scanWarning = (shelfOK: boolean, bookOK: boolean) => {
  if (!shelfOK && !bookOK) return "Не сканировались полка и книга";
  if (!shelfOK) return "Полка не сканировалась";
  return "Книга не сканировалась";
};

const formatISBN = (isbn: string) =>
  `${isbn.slice(0, 3)}-${isbn.slice(3, 4)}-${isbn.slice(4, 7)}-${isbn.slice(7, 12)}-${isbn.slice(12)}`;
