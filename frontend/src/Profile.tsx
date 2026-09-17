import { useEffect, useState } from "react";
import clsx from "clsx";
import {
  api,
  ApiError,
  type LoanDetail,
  type Me,
  type ProfileResponse,
  type ReturnReason,
} from "./api.ts";
import { BackBar } from "./Header.tsx";
import { BookCover } from "./catalog/BookVisuals.tsx";
import { bookWord, formatDueDate, pageWord } from "./catalog/presentation.ts";
import { Button, ButtonLink } from "./ui/Button.tsx";
import styles from "./Profile.module.scss";

const joinedDate = new Intl.DateTimeFormat("ru-RU", {
  day: "numeric",
  month: "long",
  year: "numeric",
});

const historyMonth = new Intl.DateTimeFormat("ru-RU", {
  month: "long",
  year: "numeric",
});

const reasonLabels: Record<ReturnReason, string> = {
  finished: "Прочитано целиком",
  too_hard: "Слишком сложно",
  boring: "Скучно",
  skipped: "Без ответа",
};

export function Profile({
  me,
  onSignOut,
}: {
  me: Me;
  onSignOut: () => void | Promise<void>;
}) {
  const [profile, setProfile] = useState<ProfileResponse>();
  const [error, setError] = useState<string | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [signingOut, setSigningOut] = useState(false);

  useEffect(() => {
    let current = true;
    api<ProfileResponse>("/profile").then(
      (result) => {
        if (current) setProfile(result);
      },
      (err: ApiError) => {
        if (current) setError(err.message);
      },
    );
    return () => {
      current = false;
    };
  }, [attempt]);

  const signOut = async () => {
    setSigningOut(true);
    await onSignOut();
  };

  return (
    <>
      <BackBar label="Каталог" note="Видно только тебе" />
      <div className={styles.profile}>
        <header className={styles.heading}>
          <h1>{me.display_name}</h1>
          {profile && (
            <p>
              {me.is_admin && "Учитель · "}в клубе с{" "}
              {joinedDate.format(new Date(profile.joined_at))}
            </p>
          )}
        </header>

        {error ? (
          <ProfileError
            message={error}
            onRetry={() => {
              setProfile(undefined);
              setError(null);
              setAttempt((value) => value + 1);
            }}
          />
        ) : profile === undefined ? (
          <p className={styles.loading} role="status">
            Загружаем профиль…
          </p>
        ) : (
          <>
            <CurrentBook loan={profile.current_loan} />
            <ReadingStats profile={profile} />
            <ReadingHistory loans={profile.history} />
          </>
        )}

        <div className={styles.signOut}>
          <Button
            variant="quiet"
            compact
            disabled={signingOut}
            onClick={() => void signOut()}
          >
            {signingOut ? "Выходим…" : "Выйти из аккаунта"}
          </Button>
        </div>
      </div>
    </>
  );
}

function CurrentBook({ loan }: { loan: LoanDetail | null }) {
  const status = loan ? dueStatus(loan.due_at) : null;
  return (
    <section className={styles.section}>
      <h2 className={styles.eyebrow}>Сейчас читаешь</h2>
      {loan && status ? (
        <>
          <div className={styles.currentBook}>
            <div className={styles.currentCover}>
              <BookCover book={loan.book} />
            </div>
            <div className={styles.currentDetails}>
              <h3>{loan.book.title}</h3>
              <p className={styles.bookMeta}>
                {loan.book.author} · {loan.book.page_count}{" "}
                {pageWord(loan.book.page_count)}
              </p>
              <div
                className={clsx(styles.due, status.overdue && styles.overdue)}
              >
                <strong>до {formatDueDate(loan.due_at)}</strong>
                <span>{status.label}</span>
              </div>
            </div>
          </div>
          <ButtonLink href="/return" variant="secondary" compact>
            Вернуть книгу
          </ButtonLink>
        </>
      ) : (
        <div className={styles.emptyCurrent}>
          <h3>Сейчас книги нет</h3>
          <p>Посмотри каталог и выбери следующую книгу с полки.</p>
          <ButtonLink href="/" variant="secondary" compact>
            Выбрать книгу
          </ButtonLink>
        </div>
      )}
    </section>
  );
}

function ReadingStats({ profile }: { profile: ProfileResponse }) {
  const { finished_books: books, pages_read: pages } = profile.stats;
  return (
    <section className={styles.section}>
      <h2 className={styles.eyebrow}>Прочитано</h2>
      <div className={styles.stats}>
        <strong>{books}</strong>
        <p>
          {bookWord(books)} целиком
          <span>
            {pages} {pageWord(pages)} по-английски
          </span>
        </p>
      </div>
    </section>
  );
}

function ReadingHistory({ loans }: { loans: LoanDetail[] }) {
  return (
    <section className={styles.historySection}>
      <h2 className={styles.eyebrow}>История</h2>
      {loans.length === 0 ? (
        <div className={styles.emptyHistory}>
          <h3>История пока пустая</h3>
          <p>Возвращённые книги появятся здесь.</p>
        </div>
      ) : (
        <div className={styles.history}>
          {loans.map((loan) => (
            <article className={styles.historyRow} key={loan.id}>
              <span
                className={clsx(styles.historySpine, styles[loan.book.level])}
                aria-hidden="true"
              />
              <div className={styles.historyBook}>
                <h3>{loan.book.title}</h3>
                <p>
                  {loan.book.page_count} {pageWord(loan.book.page_count)}
                  {loan.returned_at &&
                    ` · ${historyMonth.format(new Date(loan.returned_at))}`}
                </p>
              </div>
              <span
                className={clsx(
                  styles.reason,
                  loan.return_reason === "finished" && styles.finished,
                )}
              >
                {loan.return_reason
                  ? reasonLabels[loan.return_reason]
                  : "Без ответа"}
              </span>
            </article>
          ))}
        </div>
      )}
      <p className={styles.privacyNote}>
        Книга, которую ты не дочитал, — тоже часть истории. Никто, кроме тебя и
        учителя, её не видит.
      </p>
    </section>
  );
}

function ProfileError({
  message,
  onRetry,
}: {
  message: string;
  onRetry: () => void;
}) {
  return (
    <div className={styles.error} role="alert">
      <h2>Профиль не загрузился</h2>
      <p>{message}</p>
      <Button variant="secondary" compact onClick={onRetry}>
        Попробовать ещё раз
      </Button>
    </div>
  );
}

function dueStatus(value: string): { label: string; overdue: boolean } {
  const difference = new Date(value).getTime() - Date.now();
  if (difference >= 0) {
    const days = Math.ceil(difference / 86_400_000);
    return {
      label: days === 0 ? "сдать сегодня" : `осталось ${days} ${dayWord(days)}`,
      overdue: false,
    };
  }
  const days = Math.max(1, Math.ceil(Math.abs(difference) / 86_400_000));
  return {
    label: `срок прошёл ${days} ${dayWord(days)} назад`,
    overdue: true,
  };
}

function dayWord(value: number): string {
  const mod100 = value % 100;
  const mod10 = value % 10;
  if (mod100 >= 11 && mod100 <= 14) return "дней";
  if (mod10 === 1) return "день";
  if (mod10 >= 2 && mod10 <= 4) return "дня";
  return "дней";
}
