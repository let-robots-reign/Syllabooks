import { useEffect, useMemo, useState } from "react";
import clsx from "clsx";
import {
  api,
  ApiError,
  type AdminStats,
  type AdminUser,
  type AdminUsersResponse,
  type IssuedCodeResponse,
} from "../api.ts";
import { formatDueDate } from "../catalog/presentation.ts";
import { AdminShell } from "./AdminShell.tsx";
import styles from "./AdminUsers.module.scss";

type Confirmation = {
  id: string;
  action: "reissue" | "reset" | "ban";
} | null;

type IssuedCode = { name: string; code: string; rotated: boolean } | null;

export function AdminUsers() {
  const [stats, setStats] = useState<AdminStats>();
  const [data, setData] = useState<AdminUsersResponse>();
  const [query, setQuery] = useState("");
  const [showAll, setShowAll] = useState(false);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  const [editing, setEditing] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const [confirmation, setConfirmation] = useState<Confirmation>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [rowError, setRowError] = useState<{ id: string; message: string }>();
  const [issued, setIssued] = useState<IssuedCode>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    let current = true;
    Promise.all([
      api<AdminStats>("/admin/stats"),
      api<AdminUsersResponse>("/admin/users"),
    ]).then(
      ([nextStats, users]) => {
        if (!current) return;
        setStats(nextStats);
        setData(users);
      },
      (err: ApiError) => {
        if (current) setError(err.message);
      },
    );
    return () => {
      current = false;
    };
  }, []);

  const filtered = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase("ru");
    return (data?.users ?? []).filter((user) =>
      user.display_name.toLocaleLowerCase("ru").includes(needle),
    );
  }, [data, query]);
  const visible = showAll ? filtered : filtered.slice(0, 8);
  const asOf = data ? new Date(data.as_of).getTime() : 0;
  const mutationBusy = busy !== null;

  const createUser = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy("create");
    setError(null);
    try {
      const result = await api<IssuedCodeResponse>("/admin/users", {
        method: "POST",
        body: { display_name: newName },
      });
      const now = new Date().toISOString();
      const user: AdminUser = {
        id: result.id,
        display_name: result.display_name ?? newName.trim(),
        login_method: "code",
        code: result.code,
        has_password: false,
        status: "approved",
        created_at: now,
        current_loan: null,
        finished_count: 0,
        abandoned_count: 0,
      };
      setData((current) =>
        current
          ? {
              ...current,
              users: [user, ...current.users],
              summary: {
                ...current.summary,
                without_book: current.summary.without_book + 1,
                never_borrowed: current.summary.never_borrowed + 1,
              },
            }
          : current,
      );
      setStats((current) =>
        current ? { ...current, users: current.users + 1 } : current,
      );
      setIssued({ name: user.display_name, code: result.code, rotated: false });
      setCopied(false);
      setNewName("");
      setCreating(false);
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setBusy(null);
    }
  };

  const saveName = async (user: AdminUser) => {
    setBusy(`rename:${user.id}`);
    setRowError(undefined);
    try {
      const result = await api<{ id: string; display_name: string }>(
        `/admin/users/${user.id}`,
        { method: "PATCH", body: { display_name: editName } },
      );
      updateUser(user.id, (current) => ({
        ...current,
        display_name: result.display_name,
      }));
      setEditing(null);
    } catch (err) {
      setRowError({ id: user.id, message: (err as ApiError).message });
    } finally {
      setBusy(null);
    }
  };

  const runAction = async (
    user: AdminUser,
    action: NonNullable<Confirmation>["action"],
  ) => {
    setBusy(`${action}:${user.id}`);
    setRowError(undefined);
    try {
      if (action === "reissue") {
        const result = await api<IssuedCodeResponse>(
          `/admin/users/${user.id}/reissue-code`,
          { method: "POST" },
        );
        updateUser(user.id, (current) => ({ ...current, code: result.code }));
        setIssued({
          name: user.display_name,
          code: result.code,
          rotated: true,
        });
        setCopied(false);
      } else {
        await api(
          `/admin/users/${user.id}/${action === "reset" ? "reset-password" : "ban"}`,
          {
            method: "POST",
          },
        );
        updateUser(user.id, (current) =>
          action === "reset"
            ? { ...current, has_password: false }
            : { ...current, status: "banned" },
        );
      }
      setConfirmation(null);
    } catch (err) {
      setRowError({ id: user.id, message: (err as ApiError).message });
    } finally {
      setBusy(null);
    }
  };

  const updateUser = (id: string, change: (user: AdminUser) => AdminUser) => {
    setData((current) =>
      current
        ? {
            ...current,
            users: current.users.map((user) =>
              user.id === id ? change(user) : user,
            ),
          }
        : current,
    );
  };

  const copyCode = async () => {
    if (!issued) return;
    try {
      await navigator.clipboard.writeText(issued.code);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  return (
    <AdminShell
      active="users"
      stats={stats}
      actions={
        <>
          <label className={styles.search}>
            <span className={styles.visuallyHidden}>Поиск по имени</span>
            <input
              type="search"
              value={query}
              placeholder="Поиск по имени"
              onChange={(event) => {
                setQuery(event.target.value);
                setShowAll(false);
              }}
            />
          </label>
          <button
            type="button"
            className={styles.add}
            disabled={mutationBusy}
            onClick={() => {
              setCreating((current) => !current);
              setError(null);
            }}
          >
            Выдать код ученика
          </button>
        </>
      }
    >
      {creating && (
        <form className={styles.createPanel} onSubmit={createUser}>
          <div>
            <span className={styles.eyebrow}>Новый ученик</span>
            <p>
              Имя можно исправить позже. Пароль ученик придумает при первом
              входе.
            </p>
          </div>
          <label>
            <span>Имя ученика</span>
            <input
              autoFocus
              value={newName}
              onChange={(event) => setNewName(event.target.value)}
            />
          </label>
          <button type="submit" disabled={mutationBusy}>
            {busy === "create" ? "Создаём…" : "Создать и показать код"}
          </button>
          <button type="button" onClick={() => setCreating(false)}>
            Отмена
          </button>
        </form>
      )}

      {issued && (
        <section className={styles.issued} aria-live="polite">
          <div>
            <span className={styles.eyebrow}>Код для {issued.name}</span>
            <strong>{issued.code}</strong>
          </div>
          <p>
            {issued.rotated
              ? "Запиши или передай новый код ученику. Старый код больше не работает."
              : "Запиши или передай этот код ученику. Пароль он придумает при первом входе."}
          </p>
          <button type="button" onClick={copyCode}>
            {copied ? "Скопировано" : "Скопировать"}
          </button>
          <button type="button" onClick={() => setIssued(null)}>
            Закрыть
          </button>
        </section>
      )}

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {data && <Summary summary={data.summary} />}

      {data === undefined && !error ? (
        <p className={styles.state}>Загрузка учеников…</p>
      ) : data && data.users.length === 0 ? (
        <div className={styles.empty}>
          <h1>Учеников пока нет</h1>
          <p>Выдай первый код или дождись входа через Yandex или VK.</p>
        </div>
      ) : data && filtered.length === 0 ? (
        <div className={styles.empty}>
          <h1>Никого не нашли</h1>
          <p>Проверь имя или очисти строку поиска.</p>
        </div>
      ) : (
        data && (
          <>
            <div className={styles.tableWrap}>
              <div className={styles.table}>
                <div className={clsx(styles.row, styles.tableHead)}>
                  <span>Ученик</span>
                  <span>Читает сейчас</span>
                  <span>Срок</span>
                  <span>Прочитано</span>
                  <span>Бросил</span>
                  <span>Действия</span>
                </div>
                {visible.map((user) => (
                  <UserRow
                    key={user.id}
                    user={user}
                    asOf={asOf}
                    editing={editing === user.id}
                    editName={editName}
                    confirmation={confirmation}
                    busy={busy}
                    mutationsDisabled={mutationBusy}
                    error={rowError?.id === user.id ? rowError.message : null}
                    onEditName={setEditName}
                    onStartEdit={() => {
                      setEditing(user.id);
                      setEditName(user.display_name);
                      setConfirmation(null);
                      setRowError(undefined);
                    }}
                    onCancelEdit={() => setEditing(null)}
                    onSaveName={() => saveName(user)}
                    onConfirm={(action) => {
                      setConfirmation({ id: user.id, action });
                      setEditing(null);
                      setRowError(undefined);
                    }}
                    onCancelConfirmation={() => setConfirmation(null)}
                    onRunAction={(action) => runAction(user, action)}
                  />
                ))}
              </div>
            </div>
            <footer className={styles.footer}>
              <span>
                Показаны {visible.length} из {filtered.length} · сначала новые
              </span>
              {!showAll && filtered.length > 8 && (
                <button type="button" onClick={() => setShowAll(true)}>
                  Показать все {filtered.length} →
                </button>
              )}
            </footer>
          </>
        )
      )}
    </AdminShell>
  );
}

function Summary({ summary }: { summary: AdminUsersResponse["summary"] }) {
  return (
    <section className={styles.summary} aria-label="Сводка по ученикам">
      <SummaryItem
        value={summary.finished_books}
        label="книг прочитано классом"
        accent="summaryGreen"
      />
      <SummaryItem value={summary.reading_now} label="читают сейчас" />
      <SummaryItem value={summary.without_book} label="без книги на руках" />
      <SummaryItem
        value={summary.never_borrowed}
        label="ни разу не брали книгу"
        accent="summaryRed"
      />
    </section>
  );
}

function SummaryItem({
  value,
  label,
  accent,
}: {
  value: number;
  label: string;
  accent?: "summaryGreen" | "summaryRed";
}) {
  return (
    <div>
      <strong className={accent ? styles[accent] : undefined}>{value}</strong>
      <span>{label}</span>
    </div>
  );
}

function UserRow({
  user,
  asOf,
  editing,
  editName,
  confirmation,
  busy,
  mutationsDisabled,
  error,
  onEditName,
  onStartEdit,
  onCancelEdit,
  onSaveName,
  onConfirm,
  onCancelConfirmation,
  onRunAction,
}: {
  user: AdminUser;
  asOf: number;
  editing: boolean;
  editName: string;
  confirmation: Confirmation;
  busy: string | null;
  mutationsDisabled: boolean;
  error: string | null;
  onEditName: (name: string) => void;
  onStartEdit: () => void;
  onCancelEdit: () => void;
  onSaveName: () => void;
  onConfirm: (action: NonNullable<Confirmation>["action"]) => void;
  onCancelConfirmation: () => void;
  onRunAction: (action: NonNullable<Confirmation>["action"]) => void;
}) {
  const overdue = Boolean(
    user.current_loan && new Date(user.current_loan.due_at).getTime() < asOf,
  );
  const activeConfirmation =
    confirmation?.id === user.id ? confirmation.action : null;
  const rowBusy = busy?.endsWith(`:${user.id}`) ?? false;

  return (
    <div
      className={clsx(
        styles.row,
        overdue && styles.overdueRow,
        user.status === "banned" && styles.bannedRow,
      )}
    >
      <div className={styles.student}>
        {editing ? (
          <input
            value={editName}
            onChange={(event) => onEditName(event.target.value)}
          />
        ) : (
          <strong>{user.display_name}</strong>
        )}
        <span>{loginDescription(user)}</span>
      </div>
      {user.current_loan ? (
        <div className={styles.book}>
          <span
            className={clsx(styles.spine, styles[user.current_loan.book.level])}
          />
          <strong>{user.current_loan.book.title}</strong>
        </div>
      ) : (
        <span className={styles.muted}>—</span>
      )}
      <span className={clsx(styles.due, overdue && styles.overdue)}>
        {user.current_loan ? formatDueDate(user.current_loan.due_at) : "—"}
      </span>
      <span>
        {user.finished_count} {bookWord(user.finished_count)}
      </span>
      <span className={styles.muted}>{user.abandoned_count}</span>
      <div className={styles.actions}>
        {editing ? (
          <>
            <button
              type="button"
              disabled={mutationsDisabled}
              onClick={onSaveName}
            >
              Сохранить
            </button>
            <button type="button" disabled={rowBusy} onClick={onCancelEdit}>
              Отмена
            </button>
          </>
        ) : activeConfirmation ? (
          <>
            <span>{confirmationText(activeConfirmation)}</span>
            <button
              type="button"
              className={
                activeConfirmation === "ban" ? styles.danger : styles.accept
              }
              disabled={mutationsDisabled}
              onClick={() => onRunAction(activeConfirmation)}
            >
              {rowBusy ? "Сохраняем…" : "Подтвердить"}
            </button>
            <button
              type="button"
              disabled={rowBusy}
              onClick={onCancelConfirmation}
            >
              Отмена
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              disabled={mutationsDisabled}
              onClick={onStartEdit}
            >
              Имя
            </button>
            {user.login_method === "code" && user.status !== "banned" && (
              <>
                <button
                  type="button"
                  disabled={mutationsDisabled}
                  onClick={() => onConfirm("reissue")}
                >
                  Новый код
                </button>
                <button
                  type="button"
                  onClick={() => onConfirm("reset")}
                  disabled={mutationsDisabled || !user.has_password}
                >
                  Сбросить пароль
                </button>
              </>
            )}
            {user.status === "banned" ? (
              <span className={styles.banned}>Заблокирован</span>
            ) : (
              <button
                type="button"
                className={styles.danger}
                disabled={mutationsDisabled}
                onClick={() => onConfirm("ban")}
              >
                Заблокировать
              </button>
            )}
          </>
        )}
        {error && (
          <span className={styles.rowError} role="alert">
            {error}
          </span>
        )}
      </div>
    </div>
  );
}

const loginDescription = (user: AdminUser) => {
  if (user.login_method === "code") {
    return `код ${user.code ?? "—"} · ${user.has_password ? "пароль задан" : "ждёт первого входа"}`;
  }
  return user.login_method === "yandex" ? "Yandex" : "VK";
};

const confirmationText = (action: NonNullable<Confirmation>["action"]) => {
  if (action === "reissue") return "Старый код перестанет работать.";
  if (action === "reset") return "Все устройства выйдут из аккаунта.";
  return "Вход будет заблокирован на всех устройствах.";
};

const bookWord = (count: number) => {
  const mod100 = count % 100;
  const mod10 = count % 10;
  if (mod100 >= 11 && mod100 <= 14) return "книг";
  if (mod10 === 1) return "книга";
  if (mod10 >= 2 && mod10 <= 4) return "книги";
  return "книг";
};
