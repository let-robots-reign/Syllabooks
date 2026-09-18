import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
} from "react";
import QrScanner from "qr-scanner";
import { useNavigate } from "react-router-dom";
import {
  api,
  ApiError,
  type FinishCelebration,
  type LoanDetail,
  type ReturnEvidence,
  type ReturnMethod,
  type ReturnReason,
  type ReturnReasonResponse,
} from "../api.ts";
import { rememberAuthReturnPath } from "../authResume.ts";
import {
  cameraRecognitionTimeoutMs,
  cameraRecoverySteps,
  type CameraFailure,
} from "../cameraRecovery.ts";
import { BookCover } from "../catalog/BookVisuals.tsx";
import {
  booksReadWord,
  formatDueDate,
  pageWord,
} from "../catalog/presentation.ts";
import { CameraViewport } from "../scan/Scan.tsx";
import { formatIsbn, isbnDigits, manualIsbnError } from "../scan/isbn.ts";
import { Button } from "../ui/Button.tsx";
import { Link } from "../ui/Link.tsx";
import styles from "./Return.module.scss";

type Phase =
  | "loading"
  | "load-error"
  | "pending-reason-error"
  | "session-expired"
  | "no-loan"
  | "shelf-camera"
  | "shelf-manual"
  | "book-camera"
  | "book-manual"
  | "confirm"
  | "returning"
  | "return-error"
  | "reason"
  | "saving-reason"
  | "celebration";

const skippedEvidence = (): ReturnEvidence => ({
  method: "skipped",
  value: "",
});

type PendingReason = {
  loanId: string;
  reason: ReturnReason;
};

const pendingReasonKey = "syllabooks:pending-return-reason";

const finishDate = new Intl.DateTimeFormat("ru-RU", {
  day: "numeric",
  month: "long",
  year: "numeric",
});

const readPendingReason = (): PendingReason | null => {
  try {
    const value = sessionStorage.getItem(pendingReasonKey);
    if (!value) return null;
    const parsed = JSON.parse(value) as Partial<PendingReason>;
    if (
      typeof parsed.loanId !== "string" ||
      !["finished", "too_hard", "boring", "skipped"].includes(
        parsed.reason ?? "",
      )
    ) {
      sessionStorage.removeItem(pendingReasonKey);
      return null;
    }
    return parsed as PendingReason;
  } catch {
    try {
      sessionStorage.removeItem(pendingReasonKey);
    } catch {
      // Storage can be disabled; the ordinary in-memory flow still works.
    }
    return null;
  }
};

const savePendingReason = (pending: PendingReason): void => {
  try {
    sessionStorage.setItem(pendingReasonKey, JSON.stringify(pending));
  } catch {
    // Best-effort recovery only; saving the reason must still be attempted.
  }
};

const clearPendingReason = (): void => {
  try {
    sessionStorage.removeItem(pendingReasonKey);
  } catch {
    // Nothing else is required after the server has accepted the reason.
  }
};

const initialShelfPhase = (): Phase =>
  window.matchMedia("(min-width: 481px)").matches
    ? "shelf-manual"
    : "shelf-camera";

const cameraErrorName = (error: unknown): string => {
  if (error && typeof error === "object" && "name" in error) {
    return String(error.name);
  }
  return "";
};

function ReturnHeader({ disabled = false }: { disabled?: boolean }) {
  return (
    <header className={styles.header}>
      <span>Вернуть книгу</span>
      {disabled ? (
        <button type="button" aria-label="Возврат выполняется" disabled>
          ×
        </button>
      ) : (
        <Link href="/" aria-label="Закрыть возврат">
          ×
        </Link>
      )}
    </header>
  );
}

function StepProgress({ step }: { step: 1 | 2 }) {
  return (
    <div className={styles.progress} aria-label={`Шаг ${step} из 2`}>
      <span className={step === 2 ? styles.completed : styles.current} />
      <span className={step === 2 ? styles.current : undefined} />
      <b>ШАГ {step} ИЗ 2</b>
    </div>
  );
}

function QrViewport({
  onDetected,
  onFailure,
  onUnreadable,
  paused,
}: {
  onDetected: (value: string) => void;
  onFailure: (failure: CameraFailure) => void;
  onUnreadable: () => void;
  paused: boolean;
}) {
  const video = useRef<HTMLVideoElement>(null);
  const scanner = useRef<QrScanner | null>(null);
  const previousPaused = useRef(paused);
  const lastResult = useRef({ value: "", at: 0 });
  const unreadableTimer = useRef<number | undefined>(undefined);
  const scanCycle = useRef(0);
  const scanSettled = useRef(true);
  const [starting, setStarting] = useState(true);

  useEffect(() => {
    if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) {
      onFailure("unavailable");
      return;
    }
    if (!video.current) return;

    let disposed = false;
    const cycle = ++scanCycle.current;
    scanSettled.current = false;
    const qrScanner = new QrScanner(
      video.current,
      (result) => {
        if (disposed || scanner.current !== qrScanner || scanSettled.current) {
          return;
        }
        const now = Date.now();
        if (
          result.data === lastResult.current.value &&
          now - lastResult.current.at < 1500
        ) {
          return;
        }
        scanSettled.current = true;
        window.clearTimeout(unreadableTimer.current);
        lastResult.current = { value: result.data, at: now };
        onDetected(result.data);
      },
      {
        preferredCamera: "environment",
        maxScansPerSecond: 10,
        returnDetailedScanResult: true,
      },
    );
    scanner.current = qrScanner;
    qrScanner.start().then(
      () => {
        if (disposed) return;
        setStarting(false);
        unreadableTimer.current = window.setTimeout(() => {
          if (
            disposed ||
            scanner.current !== qrScanner ||
            scanCycle.current !== cycle ||
            scanSettled.current
          ) {
            return;
          }
          scanSettled.current = true;
          void qrScanner.pause(true);
          onUnreadable();
        }, cameraRecognitionTimeoutMs);
      },
      (error: unknown) => {
        if (disposed) return;
        const name = cameraErrorName(error);
        onFailure(
          name === "NotAllowedError" || name === "SecurityError"
            ? "denied"
            : "unavailable",
        );
      },
    );

    return () => {
      disposed = true;
      scanCycle.current += 1;
      scanSettled.current = true;
      window.clearTimeout(unreadableTimer.current);
      scanner.current = null;
      qrScanner.destroy();
    };
  }, [onDetected, onFailure, onUnreadable]);

  useEffect(() => {
    if (previousPaused.current === paused) return;
    previousPaused.current = paused;
    const qrScanner = scanner.current;
    if (!qrScanner) return;
    const cycle = ++scanCycle.current;
    let cancelled = false;
    window.clearTimeout(unreadableTimer.current);
    scanSettled.current = paused;

    if (paused) {
      void qrScanner.pause();
      return () => {
        cancelled = true;
      };
    }

    void qrScanner.start().then(
      () => {
        if (
          cancelled ||
          scanner.current !== qrScanner ||
          scanCycle.current !== cycle
        ) {
          return;
        }
        unreadableTimer.current = window.setTimeout(() => {
          if (
            cancelled ||
            scanner.current !== qrScanner ||
            scanCycle.current !== cycle ||
            scanSettled.current
          ) {
            return;
          }
          scanSettled.current = true;
          void qrScanner.pause(true);
          onUnreadable();
        }, cameraRecognitionTimeoutMs);
      },
      (error: unknown) => {
        if (
          cancelled ||
          scanner.current !== qrScanner ||
          scanCycle.current !== cycle
        ) {
          return;
        }
        scanSettled.current = true;
        const name = cameraErrorName(error);
        onFailure(
          name === "NotAllowedError" || name === "SecurityError"
            ? "denied"
            : "unavailable",
        );
      },
    );

    return () => {
      cancelled = true;
      if (scanCycle.current === cycle) {
        scanCycle.current += 1;
        scanSettled.current = true;
        window.clearTimeout(unreadableTimer.current);
      }
    };
  }, [onFailure, onUnreadable, paused]);

  return (
    <div className={styles.viewport}>
      <video ref={video} muted playsInline />
      {starting && <p>Открываем камеру…</p>}
      <i className={styles.topLeft} />
      <i className={styles.topRight} />
      <i className={styles.bottomLeft} />
      <i className={styles.bottomRight} />
      <div className={styles.scanLine} />
    </div>
  );
}

function CameraRecovery({
  failure,
  onManual,
  manualLabel = "Ввести код вручную",
}: {
  failure: CameraFailure;
  onManual: () => void;
  manualLabel?: string;
}) {
  const steps = cameraRecoverySteps();
  const denied = failure === "denied";

  return (
    <div className={styles.cameraRecovery} role="alert">
      <strong>{denied ? "Камере не дали доступ" : "Камера недоступна"}</strong>
      <p>Код можно ввести руками или вернуть книгу без сканирования.</p>
      {denied && (
        <ol>
          {steps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
      )}
      <Button variant="secondary" onClick={onManual}>
        {manualLabel}
      </Button>
    </div>
  );
}

function UnreadableCode({ kind }: { kind: "shelf" | "book" }) {
  return (
    <div className={styles.cameraRecovery} role="alert">
      <strong>
        {kind === "shelf" ? "Код полки не распознан" : "Штрих-код не распознан"}
      </strong>
      <p>
        {kind === "shelf"
          ? "Добавь света и попробуй ещё раз — или введи код под QR вручную."
          : "Добавь света, протри камеру и попробуй ещё раз — или введи ISBN вручную."}
      </p>
    </div>
  );
}

function ScanFooter({
  manualLabel,
  onManual,
  onRetry,
  onSkip,
  disabled = false,
}: {
  manualLabel: string;
  onManual: () => void;
  onRetry?: () => void;
  onSkip: () => void;
  disabled?: boolean;
}) {
  return (
    <footer className={styles.scanFooter}>
      {onRetry && <Button onClick={onRetry}>Ещё раз камерой</Button>}
      <Button variant="secondary" onClick={onManual} disabled={disabled}>
        {manualLabel}
      </Button>
      <button
        type="button"
        className={styles.skipScan}
        onClick={onSkip}
        disabled={disabled}
      >
        Вернуть без сканирования
      </button>
    </footer>
  );
}

function ShelfCameraScreen({
  error,
  busy,
  failure,
  unreadable,
  onDetected,
  onFailure,
  onUnreadable,
  onRetryCamera,
  onManual,
  onSkip,
}: {
  error: string | null;
  busy: boolean;
  failure: CameraFailure | null;
  unreadable: boolean;
  onDetected: (value: string) => void;
  onFailure: (failure: CameraFailure) => void;
  onUnreadable: () => void;
  onRetryCamera: () => void;
  onManual: () => void;
  onSkip: () => void;
}) {
  return (
    <div className={styles.cameraScreen}>
      <div className={styles.cameraTop}>
        <ReturnHeader disabled={busy} />
        <StepProgress step={1} />
        <h1>Код рядом с полкой</h1>
        <p>Он приклеен на дверце шкафа у окна. Сначала полка, потом книга.</p>
      </div>
      {unreadable ? (
        <UnreadableCode kind="shelf" />
      ) : failure ? (
        <CameraRecovery failure={failure} onManual={onManual} />
      ) : (
        <QrViewport
          onDetected={onDetected}
          onFailure={onFailure}
          onUnreadable={onUnreadable}
          paused={busy}
        />
      )}
      {busy && <p className={styles.checking}>Проверяем код…</p>}
      {error && <p className={styles.cameraError}>{error}</p>}
      <ScanFooter
        manualLabel="Ввести код вручную"
        onManual={onManual}
        onRetry={unreadable ? onRetryCamera : undefined}
        onSkip={onSkip}
        disabled={busy}
      />
    </div>
  );
}

function ManualShelfScreen({
  error,
  busy,
  onSubmit,
  onCamera,
  onSkip,
}: {
  error: string | null;
  busy: boolean;
  onSubmit: (value: string) => void;
  onCamera: () => void;
  onSkip: () => void;
}) {
  const [value, setValue] = useState("");

  return (
    <div className={styles.paperScreen}>
      <ReturnHeader disabled={busy} />
      <StepProgress step={1} />
      <form
        className={styles.manualForm}
        onSubmit={(event) => {
          event.preventDefault();
          if (value.trim()) onSubmit(value);
        }}
      >
        <h1>Введи код полки</h1>
        <p>Он напечатан под QR-кодом на дверце шкафа.</p>
        <label htmlFor="shelf-code">Код полки</label>
        <input
          id="shelf-code"
          value={value}
          onChange={(event) => setValue(event.target.value)}
          autoCapitalize="characters"
          autoComplete="off"
          spellCheck={false}
          disabled={busy}
        />
        {error && <div className={styles.formError}>{error}</div>}
        <Button type="submit" disabled={!value.trim() || busy}>
          {busy ? "Проверяем…" : "Дальше"}
        </Button>
        <Button variant="secondary" onClick={onCamera} disabled={busy}>
          Сканировать камерой
        </Button>
        <button
          type="button"
          className={styles.skipScan}
          onClick={onSkip}
          disabled={busy}
        >
          Вернуть без сканирования
        </button>
      </form>
    </div>
  );
}

function BookCameraScreen({
  loan,
  error,
  failure,
  unreadable,
  onDetected,
  onFailure,
  onUnreadable,
  onRetryCamera,
  onManual,
  onSkip,
}: {
  loan: LoanDetail;
  error: string | null;
  failure: CameraFailure | null;
  unreadable: boolean;
  onDetected: (value: string) => void;
  onFailure: (failure: CameraFailure) => void;
  onUnreadable: () => void;
  onRetryCamera: () => void;
  onManual: () => void;
  onSkip: () => void;
}) {
  return (
    <div className={styles.cameraScreen}>
      <div className={styles.cameraTop}>
        <ReturnHeader />
        <StepProgress step={2} />
        <h1>Теперь штрих-код книги</h1>
        <p>{loan.book.title} — задняя обложка, внизу справа.</p>
      </div>
      {unreadable ? (
        <UnreadableCode kind="book" />
      ) : failure ? (
        <CameraRecovery
          failure={failure}
          onManual={onManual}
          manualLabel="Ввести ISBN вручную"
        />
      ) : (
        <CameraViewport
          onDetected={onDetected}
          onFailure={onFailure}
          onUnreadable={onUnreadable}
        />
      )}
      {error && <p className={styles.cameraError}>{error}</p>}
      <ScanFooter
        manualLabel="Ввести ISBN вручную"
        onManual={onManual}
        onRetry={unreadable ? onRetryCamera : undefined}
        onSkip={onSkip}
      />
    </div>
  );
}

function ManualBookScreen({
  loan,
  error,
  onSubmit,
  onCamera,
  onSkip,
}: {
  loan: LoanDetail;
  error: string | null;
  onSubmit: (value: string) => void;
  onCamera: () => void;
  onSkip: () => void;
}) {
  const [digits, setDigits] = useState("");
  const isbnError = manualIsbnError(digits);
  const update = (event: ChangeEvent<HTMLInputElement>) =>
    setDigits(isbnDigits(event.target.value));

  return (
    <div className={styles.paperScreen}>
      <ReturnHeader />
      <StepProgress step={2} />
      <form
        className={styles.manualForm}
        onSubmit={(event: FormEvent) => {
          event.preventDefault();
          if (digits.length === 13 && !isbnError) onSubmit(digits);
        }}
      >
        <h1>ISBN книги</h1>
        <p>
          Введи 13 цифр под штрих-кодом на задней обложке «{loan.book.title}».
        </p>
        <label htmlFor="return-isbn">ISBN · {digits.length} из 13</label>
        <input
          id="return-isbn"
          value={formatIsbn(digits)}
          onChange={update}
          inputMode="numeric"
          autoComplete="off"
          spellCheck={false}
          placeholder="978-___-___-___-_"
        />
        {(isbnError || error) && (
          <div className={styles.formError}>{isbnError ?? error}</div>
        )}
        <Button type="submit" disabled={digits.length !== 13 || !!isbnError}>
          Вернуть книгу
        </Button>
        <Button variant="secondary" onClick={onCamera}>
          Сканировать камерой
        </Button>
        <button type="button" className={styles.skipScan} onClick={onSkip}>
          Вернуть без сканирования
        </button>
      </form>
    </div>
  );
}

function LoanCard({ loan }: { loan: LoanDetail }) {
  return (
    <div className={styles.loanCard}>
      <div className={styles.cardEyebrow}>Ты возвращаешь</div>
      <div className={styles.cardBook}>
        <div className={styles.cardCover}>
          <BookCover book={loan.book} />
        </div>
        <div>
          <h2>{loan.book.title}</h2>
          <p>{loan.book.author}</p>
          <p className={styles.cardDates}>
            Взято {formatDueDate(loan.taken_at)}
            <br />
            Срок — до {formatDueDate(loan.due_at)}
          </p>
        </div>
      </div>
    </div>
  );
}

function ConfirmScreen({
  loan,
  busy,
  onBack,
  onConfirm,
}: {
  loan: LoanDetail;
  busy: boolean;
  onBack: () => void;
  onConfirm: () => void;
}) {
  const [confirmed, setConfirmed] = useState(false);

  return (
    <div className={styles.paperScreen}>
      <div className={styles.confirmNav}>
        <button type="button" onClick={onBack} disabled={busy}>
          ← Назад
        </button>
        <Link href="/" aria-label="Закрыть возврат">
          ×
        </Link>
      </div>
      <section className={styles.confirmContent}>
        <h1>Код не сканируется</h1>
        <p className={styles.confirmLead}>
          Ничего страшного. Поставь книгу на полку и подтверди вручную — учитель
          сверит на следующем уроке.
        </p>
        <LoanCard loan={loan} />
        <label className={styles.confirmCheck}>
          <input
            type="checkbox"
            checked={confirmed}
            onChange={(event) => setConfirmed(event.target.checked)}
          />
          <span>Книга стоит на полке в шкафу у окна</span>
        </label>
      </section>
      <footer className={styles.confirmFooter}>
        <Button onClick={onConfirm} disabled={!confirmed || busy}>
          {busy ? "Возвращаем…" : "Подтвердить возврат"}
        </Button>
      </footer>
    </div>
  );
}

function PendingScreen() {
  return (
    <div className={styles.paperScreen}>
      <ReturnHeader disabled />
      <div className={styles.pending} role="status">
        <div aria-hidden="true">
          <span />
          <span />
          <span />
        </div>
        <h1>Возвращаем книгу…</h1>
        <p>Это займёт пару секунд.</p>
      </div>
    </div>
  );
}

const returnReasons: Array<{
  value: ReturnReason;
  label: string;
  note?: string;
}> = [
  {
    value: "finished",
    label: "Дочитал до конца",
    note: "пойдёт в счёт класса",
  },
  { value: "too_hard", label: "Слишком сложно" },
  { value: "boring", label: "Скучно" },
  { value: "skipped", label: "Не хочу отвечать" },
];

function ReasonScreen({
  loan,
  saving,
  error,
  onChoose,
}: {
  loan: LoanDetail;
  saving: boolean;
  error: string | null;
  onChoose: (reason: ReturnReason) => void;
}) {
  const returnedAt = loan.returned_at ?? new Date().toISOString();
  const days = Math.max(
    1,
    Math.ceil(
      (new Date(returnedAt).getTime() - new Date(loan.taken_at).getTime()) /
        86_400_000,
    ),
  );

  return (
    <div className={styles.reasonScreen}>
      <section className={styles.reasonContent}>
        <div className={styles.successEyebrow}>Книга вернулась на полку</div>
        <h1>Как прошло с {loan.book.title}?</h1>
        <p className={styles.reasonLead}>
          Один тап — и всё. Это видит только учитель, чтобы понимать, что
          ставить на полку дальше.
        </p>
        <div className={styles.reasonBook}>
          <div className={styles.reasonCover}>
            <BookCover book={loan.book} />
          </div>
          <div>
            <h2>{loan.book.title}</h2>
            <p>
              {loan.book.author} · {loan.book.page_count}{" "}
              {pageWord(loan.book.page_count)}
            </p>
            <p className={styles.reasonDates}>
              Была у тебя {days} {dayWord(days)}
              <br />
              {formatDueDate(loan.taken_at)} — {formatDueDate(returnedAt)}
            </p>
          </div>
        </div>
      </section>
      <footer className={styles.reasonActions}>
        {returnReasons.map((reason) => (
          <button
            type="button"
            key={reason.value}
            className={
              reason.value === "finished"
                ? styles.finishedReason
                : reason.value === "skipped"
                  ? styles.skipReason
                  : styles.reasonButton
            }
            disabled={saving}
            onClick={() => onChoose(reason.value)}
          >
            {reason.value === "finished" && <i aria-hidden="true" />}
            <span>
              <strong>{reason.label}</strong>
              {reason.note && <small>{reason.note}</small>}
            </span>
          </button>
        ))}
        {error && <p className={styles.reasonError}>{error}</p>}
      </footer>
    </div>
  );
}

function CelebrationScreen({
  loan,
  celebration,
  onNext,
  onLater,
}: {
  loan: LoanDetail;
  celebration: FinishCelebration;
  onNext: () => void;
  onLater: () => void;
}) {
  const currentCount = celebration.class_finished_count;
  const previousCount = Math.max(0, currentCount - 1);
  const returnedAt = loan.returned_at ?? new Date().toISOString();

  return (
    <div className={styles.celebrationScreen}>
      <div className={styles.celebrationContent}>
        <div className={styles.bookplate}>Книжный клуб Syllabooks</div>

        <div className={styles.celebrationCover}>
          <BookCover book={loan.book} />
        </div>
        <div className={styles.finishedStamp}>
          <span>прочитано целиком</span>
          <strong>{finishDate.format(new Date(returnedAt))}</strong>
        </div>

        <div className={styles.finishedBook}>
          <h1>{loan.book.title}</h1>
          <p>
            {loan.book.author} · {loan.book.page_count}{" "}
            {pageWord(loan.book.page_count)}
          </p>
        </div>

        <section className={styles.achievement}>
          <h2>
            {celebration.is_first_book
              ? "Твоя первая книга на английском — так держать!."
              : "Ещё одна книга на английском — ты молодец!"}
          </h2>
          <p>
            {loan.book.page_count} {pageWord(loan.book.page_count)} чужого
            языка. Дальше пойдет легче.
          </p>
        </section>

        <section
          className={styles.celebrationCounter}
          role="status"
          aria-live="polite"
          aria-label={`Счёт прочитанных книг вырос с ${previousCount} до ${currentCount} ${booksReadWord(currentCount)}`}
        >
          <div className={styles.counterEyebrow}>
            Общий счёт прочитанных книг
          </div>
          <div className={styles.counterNumbers} aria-hidden="true">
            <span>{previousCount}</span>
            <i>→</i>
            <strong>{currentCount}</strong>
            <b>{booksReadWord(currentCount)}</b>
          </div>
          <div className={styles.finishedShelf} aria-hidden="true">
            {Array.from({ length: previousCount }, (_, index) => (
              <span key={index} />
            ))}
            <span className={styles.studentSpine} />
          </div>
          <p>Последний корешок в списке — твой</p>
        </section>
      </div>

      <footer className={styles.celebrationActions}>
        <Button variant="brand" onClick={onNext}>
          Выбрать следующую книгу
        </Button>
        <Button variant="quiet" onClick={onLater}>
          Позже
        </Button>
      </footer>
    </div>
  );
}

function StateScreen({
  title,
  message,
  action,
  actionLabel,
}: {
  title: string;
  message: string;
  action?: () => void;
  actionLabel?: string;
}) {
  return (
    <div className={styles.paperScreen}>
      <ReturnHeader />
      <div className={styles.state} role="alert">
        <h1>{title}</h1>
        <p>{message}</p>
        {action && actionLabel && (
          <Button onClick={action}>{actionLabel}</Button>
        )}
        <Link href="/">Вернуться в каталог</Link>
      </div>
    </div>
  );
}

export function Return() {
  const navigate = useNavigate();
  const [phase, setPhase] = useState<Phase>("loading");
  const [loan, setLoan] = useState<LoanDetail | null>(null);
  const [shelfEvidence, setShelfEvidence] = useState<ReturnEvidence | null>(
    null,
  );
  const [pendingEvidence, setPendingEvidence] = useState<{
    shelf: ReturnEvidence;
    book: ReturnEvidence;
  } | null>(null);
  const [confirmBack, setConfirmBack] = useState<Phase>("shelf-camera");
  const [shelfBusy, setShelfBusy] = useState(false);
  const [shelfError, setShelfError] = useState<string | null>(null);
  const [bookError, setBookError] = useState<string | null>(null);
  const [reasonError, setReasonError] = useState<string | null>(null);
  const [celebration, setCelebration] = useState<FinishCelebration | null>(
    null,
  );
  const [loadError, setLoadError] = useState<string | null>(null);
  const [shelfFailure, setShelfFailure] = useState<CameraFailure | null>(null);
  const [bookFailure, setBookFailure] = useState<CameraFailure | null>(null);
  const [shelfUnreadable, setShelfUnreadable] = useState(false);
  const [bookUnreadable, setBookUnreadable] = useState(false);
  const shelfLocked = useRef(false);
  const bookLocked = useRef(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [initialPendingReason] = useState(readPendingReason);
  const pendingReason = useRef(initialPendingReason);
  const [pendingReasonRetryable, setPendingReasonRetryable] = useState(true);

  useEffect(() => {
    let current = true;
    if (pendingReason.current) {
      const pending = pendingReason.current;
      api<ReturnReasonResponse>(`/loans/${pending.loanId}/return-reason`, {
        method: "POST",
        body: { reason: pending.reason },
      }).then(
        (response) => {
          if (!current) return;
          clearPendingReason();
          if (response.celebration) {
            setLoan(response.loan);
            setCelebration(response.celebration);
            setPhase("celebration");
          } else {
            navigate("/", { replace: true });
          }
        },
        (error: unknown) => {
          if (!current) return;
          const apiError =
            error instanceof ApiError
              ? error
              : new ApiError(0, "Не получилось сохранить ответ.");
          if (apiError.status === 401) {
            rememberAuthReturnPath("/return");
            setPhase("session-expired");
          } else {
            const retryable = apiError.status === 0 || apiError.status >= 500;
            setPendingReasonRetryable(retryable);
            if (!retryable) {
              clearPendingReason();
              pendingReason.current = null;
            }
            setLoadError(apiError.message);
            setPhase("pending-reason-error");
          }
        },
      );
      return () => {
        current = false;
      };
    }
    api<LoanDetail>("/loans/current").then(
      (result) => {
        if (!current) return;
        setLoan(result);
        setPhase(initialShelfPhase());
      },
      (error: unknown) => {
        if (!current) return;
        const apiError =
          error instanceof ApiError
            ? error
            : new ApiError(0, "Не получилось связаться с сервером.");
        if (apiError.status === 401) {
          rememberAuthReturnPath("/return");
          setPhase("session-expired");
        } else if (apiError.code === "no_open_loan") setPhase("no-loan");
        else {
          setLoadError(apiError.message);
          setPhase("load-error");
        }
      },
    );
    return () => {
      current = false;
    };
  }, [loadAttempt, navigate]);

  const checkShelf = useCallback(
    (value: string, method: Exclude<ReturnMethod, "skipped">) => {
      if (!loan || shelfLocked.current) return;
      shelfLocked.current = true;
      setShelfBusy(true);
      setShelfError(null);
      api<void>(`/loans/${loan.id}/shelf-check`, {
        method: "POST",
        body: { code: value },
      }).then(
        () => {
          setShelfEvidence({ method, value });
          setShelfBusy(false);
          shelfLocked.current = false;
          setPhase(
            window.matchMedia("(min-width: 481px)").matches
              ? "book-manual"
              : "book-camera",
          );
        },
        (error: unknown) => {
          const apiError =
            error instanceof ApiError
              ? error
              : new ApiError(0, "Не получилось проверить код полки.");
          setShelfBusy(false);
          shelfLocked.current = false;
          if (apiError.status === 401) {
            rememberAuthReturnPath("/return");
            setPhase("session-expired");
          } else {
            setShelfError(apiError.message);
          }
        },
      );
    },
    [loan],
  );

  const closeLoan = useCallback(
    (shelf: ReturnEvidence, book: ReturnEvidence) => {
      if (!loan || bookLocked.current) return;
      bookLocked.current = true;
      setPendingEvidence({ shelf, book });
      setBookError(null);
      setPhase("returning");
      api<LoanDetail>(`/loans/${loan.id}/return`, {
        method: "POST",
        body: { shelf, book },
      }).then(
        (returned) => {
          setLoan(returned);
          setPendingEvidence(null);
          bookLocked.current = false;
          setPhase("reason");
        },
        (error: unknown) => {
          const apiError =
            error instanceof ApiError
              ? error
              : new ApiError(0, "Не получилось завершить возврат.");
          bookLocked.current = false;
          if (apiError.code === "wrong_book") {
            setBookError(apiError.message);
            setPhase(book.method === "manual" ? "book-manual" : "book-camera");
          } else if (apiError.code === "invalid_shelf_code") {
            setShelfError(apiError.message);
            setPhase(
              shelf.method === "manual" ? "shelf-manual" : "shelf-camera",
            );
          } else if (apiError.status === 401) {
            rememberAuthReturnPath("/return");
            setPhase("session-expired");
          } else {
            setBookError(apiError.message);
            setPhase("return-error");
          }
        },
      );
    },
    [loan],
  );

  const openConfirmation = (
    shelf: ReturnEvidence,
    book: ReturnEvidence,
    back: Phase,
  ) => {
    setPendingEvidence({ shelf, book });
    setConfirmBack(back);
    setPhase("confirm");
  };

  const chooseReason = (reason: ReturnReason) => {
    if (!loan) return;
    const pending = { loanId: loan.id, reason };
    savePendingReason(pending);
    setReasonError(null);
    setPhase("saving-reason");
    api<ReturnReasonResponse>(`/loans/${loan.id}/return-reason`, {
      method: "POST",
      body: { reason },
    }).then(
      (response) => {
        clearPendingReason();
        if (response.celebration) {
          setLoan(response.loan);
          setCelebration(response.celebration);
          setPhase("celebration");
        } else {
          navigate("/", { replace: true });
        }
      },
      (error: unknown) => {
        const apiError =
          error instanceof ApiError
            ? error
            : new ApiError(0, "Не получилось сохранить ответ.");
        if (apiError.status === 401) {
          rememberAuthReturnPath("/return");
          setPhase("session-expired");
        } else {
          if (apiError.status >= 400 && apiError.status < 500) {
            clearPendingReason();
          }
          setReasonError(apiError.message);
          setPhase("reason");
        }
      },
    );
  };

  const handleShelfDetected = useCallback(
    (value: string) => checkShelf(value, "scan"),
    [checkShelf],
  );
  const handleShelfUnreadable = useCallback(() => setShelfUnreadable(true), []);
  const retryShelfCamera = useCallback(() => setShelfUnreadable(false), []);
  const handleBookUnreadable = useCallback(() => setBookUnreadable(true), []);
  const retryBookCamera = useCallback(() => setBookUnreadable(false), []);
  const handleBookDetected = useCallback(
    (value: string) => {
      if (shelfEvidence) {
        closeLoan(shelfEvidence, { method: "scan", value });
      }
    },
    [closeLoan, shelfEvidence],
  );

  if (phase === "loading") return <PendingScreen />;
  if (phase === "load-error") {
    return (
      <StateScreen
        title="Не удалось открыть возврат"
        message={loadError ?? "Проверь интернет и попробуй ещё раз."}
        action={() => {
          setPhase("loading");
          setLoadError(null);
          setLoadAttempt((value) => value + 1);
        }}
        actionLabel="Попробовать ещё раз"
      />
    );
  }
  if (phase === "pending-reason-error") {
    return (
      <StateScreen
        title="Не удалось сохранить ответ"
        message={loadError ?? "Проверь интернет и попробуй ещё раз."}
        action={
          pendingReasonRetryable
            ? () => {
                setPhase("loading");
                setLoadError(null);
                setLoadAttempt((value) => value + 1);
              }
            : undefined
        }
        actionLabel={pendingReasonRetryable ? "Попробовать ещё раз" : undefined}
      />
    );
  }
  if (phase === "session-expired") {
    return (
      <StateScreen
        title="Сессия закончилась"
        message="Нужно войти заново. После входа мы продолжим возврат с нужного шага."
        action={() => window.location.reload()}
        actionLabel="Войти заново"
      />
    );
  }
  if (phase === "no-loan" || !loan) {
    return (
      <StateScreen
        title="Возвращать пока нечего"
        message="У тебя сейчас нет книги на руках. Выбери следующую в каталоге."
      />
    );
  }
  if (phase === "shelf-camera") {
    return (
      <ShelfCameraScreen
        error={shelfError}
        busy={shelfBusy}
        failure={shelfFailure}
        unreadable={shelfUnreadable}
        onDetected={handleShelfDetected}
        onFailure={setShelfFailure}
        onUnreadable={handleShelfUnreadable}
        onRetryCamera={retryShelfCamera}
        onManual={() => {
          setShelfError(null);
          setShelfFailure(null);
          setShelfUnreadable(false);
          setPhase("shelf-manual");
        }}
        onSkip={() =>
          openConfirmation(skippedEvidence(), skippedEvidence(), "shelf-camera")
        }
      />
    );
  }
  if (phase === "shelf-manual") {
    return (
      <ManualShelfScreen
        error={shelfError}
        busy={shelfBusy}
        onSubmit={(value) => checkShelf(value, "manual")}
        onCamera={() => {
          setShelfError(null);
          setShelfUnreadable(false);
          setPhase("shelf-camera");
        }}
        onSkip={() =>
          openConfirmation(skippedEvidence(), skippedEvidence(), "shelf-manual")
        }
      />
    );
  }
  if (phase === "book-camera") {
    return (
      <BookCameraScreen
        loan={loan}
        error={bookError}
        failure={bookFailure}
        unreadable={bookUnreadable}
        onDetected={handleBookDetected}
        onFailure={setBookFailure}
        onUnreadable={handleBookUnreadable}
        onRetryCamera={retryBookCamera}
        onManual={() => {
          setBookError(null);
          setBookFailure(null);
          setBookUnreadable(false);
          setPhase("book-manual");
        }}
        onSkip={() => {
          if (shelfEvidence)
            openConfirmation(shelfEvidence, skippedEvidence(), "book-camera");
        }}
      />
    );
  }
  if (phase === "book-manual") {
    return (
      <ManualBookScreen
        loan={loan}
        error={bookError}
        onSubmit={(value) => {
          if (shelfEvidence)
            closeLoan(shelfEvidence, { method: "manual", value });
        }}
        onCamera={() => {
          setBookError(null);
          setBookUnreadable(false);
          setPhase("book-camera");
        }}
        onSkip={() => {
          if (shelfEvidence)
            openConfirmation(shelfEvidence, skippedEvidence(), "book-manual");
        }}
      />
    );
  }
  if (phase === "confirm" && pendingEvidence) {
    return (
      <ConfirmScreen
        loan={loan}
        busy={false}
        onBack={() => setPhase(confirmBack)}
        onConfirm={() => closeLoan(pendingEvidence.shelf, pendingEvidence.book)}
      />
    );
  }
  if (phase === "returning") return <PendingScreen />;
  if (phase === "return-error" && pendingEvidence) {
    return (
      <StateScreen
        title="Не удалось подтвердить возврат"
        message={bookError ?? "Проверь интернет и попробуй ещё раз."}
        action={() => closeLoan(pendingEvidence.shelf, pendingEvidence.book)}
        actionLabel="Повторить"
      />
    );
  }
  if (phase === "reason" || phase === "saving-reason") {
    return (
      <ReasonScreen
        loan={loan}
        saving={phase === "saving-reason"}
        error={reasonError}
        onChoose={chooseReason}
      />
    );
  }
  if (phase === "celebration" && celebration) {
    return (
      <CelebrationScreen
        loan={loan}
        celebration={celebration}
        onNext={() => navigate(`/?level=${loan.book.level}`, { replace: true })}
        onLater={() => navigate("/", { replace: true })}
      />
    );
  }
  return null;
}

function dayWord(value: number): string {
  const mod100 = value % 100;
  const mod10 = value % 10;
  if (mod100 >= 11 && mod100 <= 14) return "дней";
  if (mod10 === 1) return "день";
  if (mod10 >= 2 && mod10 <= 4) return "дня";
  return "дней";
}
