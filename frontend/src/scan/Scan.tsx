import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
} from "react";
import type { QuaggaJSResultObject, QuaggaJSStatic } from "@ericblade/quagga2";
import clsx from "clsx";
import { useNavigate } from "react-router-dom";
import { api, ApiError, type BorrowResponse } from "../api.ts";
import { bookLevel } from "../bookLevels.ts";
import { BookCover, LevelBars } from "../catalog/BookVisuals.tsx";
import { formatDueDate, pageWord } from "../catalog/presentation.ts";
import { Button, ButtonLink } from "../ui/Button.tsx";
import { Link } from "../ui/Link.tsx";
import styles from "./Scan.module.scss";
import {
  formatIsbn,
  isbnDigits,
  isValidEan13,
  manualIsbnError,
} from "./isbn.ts";

type ScanMode = "camera" | "manual";
type CameraFailure = "denied" | "unavailable";
type BorrowAttempt =
  | { status: "pending"; isbn: string; source: ScanMode }
  | { status: "success"; result: BorrowResponse }
  | { status: "error"; isbn: string; source: ScanMode; error: ApiError };

const isWideViewport = (): boolean => {
  return window.matchMedia("(min-width: 481px)").matches;
};

const cameraErrorName = (error: unknown): string => {
  if (error && typeof error === "object" && "name" in error) {
    return String(error.name);
  }
  return "";
};

const releaseScanner = async (quagga: QuaggaJSStatic): Promise<void> => {
  const stream = quagga.CameraAccess.getActiveStream();

  try {
    await quagga.stop();
  } catch {
    // stop() can reject when setup failed before Quagga became ready.
  }

  stream?.getTracks().forEach((track) => track.stop());

  try {
    await quagga.CameraAccess.release();
  } catch {
    // Tracks were stopped explicitly above; there is nothing else to recover.
  }
};

function CameraViewport({
  onDetected,
  onFailure,
}: {
  onDetected: (value: string) => void;
  onFailure: (failure: CameraFailure) => void;
}) {
  const target = useRef<HTMLDivElement>(null);
  const [starting, setStarting] = useState(true);

  useEffect(() => {
    let disposed = false;
    let quagga: QuaggaJSStatic | undefined;
    let detectedHandler: ((result: QuaggaJSResultObject) => void) | undefined;

    const start = async () => {
      if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) {
        onFailure("unavailable");
        return;
      }

      try {
        const module = await import("@ericblade/quagga2");
        quagga = module.default;
        if (disposed || !target.current) return;

        let previousCode = "";
        let matchingReads = 0;

        detectedHandler = (scan) => {
          const value = isbnDigits(scan.codeResult.code ?? "");
          if (!isValidEan13(value)) return;

          if (value === previousCode) matchingReads += 1;
          else {
            previousCode = value;
            matchingReads = 1;
          }

          if (matchingReads >= 2 && !disposed) onDetected(value);
        };

        await quagga.init({
          inputStream: {
            type: "LiveStream",
            target: target.current,
            willReadFrequently: true,
            constraints: {
              facingMode: { ideal: "environment" },
              width: { ideal: 1280 },
              height: { ideal: 720 },
            },
            area: {
              top: "12%",
              right: "7%",
              bottom: "12%",
              left: "7%",
            },
          },
          locate: true,
          frequency: 10,
          numOfWorkers: Math.min(
            2,
            Math.max(1, navigator.hardwareConcurrency || 1),
          ),
          canvas: { createOverlay: false },
          decoder: { readers: ["ean_reader"] },
          locator: { halfSample: true, patchSize: "medium" },
        });

        if (disposed) {
          await releaseScanner(quagga);
          return;
        }

        quagga.onDetected(detectedHandler);
        quagga.start();
        setStarting(false);
      } catch (error) {
        if (quagga) await releaseScanner(quagga);
        if (disposed) return;

        const name = cameraErrorName(error);
        onFailure(
          name === "NotAllowedError" || name === "SecurityError"
            ? "denied"
            : "unavailable",
        );
      }
    };

    void start();

    return () => {
      disposed = true;
      if (!quagga) return;
      if (detectedHandler) quagga.offDetected(detectedHandler);
      void releaseScanner(quagga);
    };
  }, [onDetected, onFailure]);

  return (
    <div className={styles.viewport} ref={target}>
      {starting && <p className={styles.cameraStatus}>Открываем камеру…</p>}
      <div className={clsx(styles.corner, styles.topLeft)} />
      <div className={clsx(styles.corner, styles.topRight)} />
      <div className={clsx(styles.corner, styles.bottomLeft)} />
      <div className={clsx(styles.corner, styles.bottomRight)} />
      <div className={styles.scanLine} />
      <div className={styles.barcodeHint}>штрих-код на задней обложке</div>
    </div>
  );
}

function ScreenHeader({ closeDisabled = false }: { closeDisabled?: boolean }) {
  return (
    <header className={styles.header}>
      <div className={styles.screenTitle}>Взять книгу</div>
      {closeDisabled ? (
        <button
          type="button"
          className={styles.close}
          aria-label="Выдача книги выполняется"
          disabled
        >
          ×
        </button>
      ) : (
        <Link
          href="/"
          className={styles.close}
          aria-label="Закрыть сканирование"
        >
          ×
        </Link>
      )}
    </header>
  );
}

function ModeTabs({
  mode,
  onChange,
}: {
  mode: ScanMode;
  onChange: (mode: ScanMode) => void;
}) {
  return (
    <div className={styles.tabs} role="tablist" aria-label="Способ ввода ISBN">
      <button
        type="button"
        role="tab"
        aria-selected={mode === "camera"}
        className={clsx(styles.tab, mode === "camera" && styles.activeTab)}
        onClick={() => onChange("camera")}
      >
        Камера
      </button>
      <button
        type="button"
        role="tab"
        aria-selected={mode === "manual"}
        className={clsx(styles.tab, mode === "manual" && styles.activeTab)}
        onClick={() => onChange("manual")}
      >
        Ввести ISBN
      </button>
    </div>
  );
}

function CameraScreen({
  onManual,
  onDetected,
  onFailure,
}: {
  onManual: () => void;
  onDetected: (value: string) => void;
  onFailure: (failure: CameraFailure) => void;
}) {
  return (
    <div className={clsx(styles.screen, styles.cameraScreen)}>
      <div className={styles.top}>
        <ScreenHeader />
        <ModeTabs
          mode="camera"
          onChange={(mode) => mode === "manual" && onManual()}
        />
      </div>

      <CameraViewport onDetected={onDetected} onFailure={onFailure} />
      <p className={styles.cameraHelp}>
        Держи книгу в 10–15 см, чтобы полоски попали между уголками
      </p>

      <footer className={styles.cameraFooter}>
        <p className={styles.manualHint}>
          Не сканируется или камера мешает? ISBN напечатан цифрами прямо под
          штрих-кодом — его можно ввести руками.
        </p>
        <Button variant="secondary" onClick={onManual}>
          Ввести ISBN вручную
        </Button>
      </footer>
    </div>
  );
}

const keypad = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "", "0"];

function ManualScreen({
  onCamera,
  onAccepted,
  initialValue = "",
}: {
  onCamera: () => void;
  onAccepted: (value: string) => void;
  initialValue?: string;
}) {
  const initialDigits = isbnDigits(initialValue);
  const [digits, setDigits] = useState(initialDigits);
  const lastAccepted = useRef(initialDigits);
  const error = manualIsbnError(digits);

  useEffect(() => {
    if (digits.length === 13 && !error && digits !== lastAccepted.current) {
      lastAccepted.current = digits;
      onAccepted(digits);
    }
  }, [digits, error, onAccepted]);

  const update = (value: string) => setDigits(isbnDigits(value));
  const handleChange = (event: ChangeEvent<HTMLInputElement>) =>
    update(event.target.value);
  const addDigit = (digit: string) => update(`${digits}${digit}`);
  const erase = () => setDigits((value) => value.slice(0, -1));
  const remaining = 13 - digits.length;

  return (
    <div className={clsx(styles.screen, styles.paperScreen)}>
      <div className={styles.top}>
        <ScreenHeader />
        <ModeTabs
          mode="manual"
          onChange={(mode) => mode === "camera" && onCamera()}
        />
      </div>

      <div className={styles.manualContent}>
        <div className={styles.fieldMeta}>
          <label htmlFor="manual-isbn">ISBN книги</label>
          <span>{digits.length} из 13</span>
        </div>
        <div className={clsx(styles.isbnBox, error && styles.invalidBox)}>
          <input
            id="manual-isbn"
            value={formatIsbn(digits)}
            onChange={handleChange}
            inputMode="numeric"
            enterKeyHint="done"
            autoComplete="off"
            autoCorrect="off"
            spellCheck={false}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? "manual-isbn-error" : "manual-isbn-help"}
            placeholder="978-___-___-___-_"
          />
          {remaining > 0 && (
            <span className={styles.remaining}>ещё {remaining}</span>
          )}
        </div>
        {error && (
          <p id="manual-isbn-error" className={styles.isbnError} role="alert">
            {error}
          </p>
        )}

        <div id="manual-isbn-help" className={styles.isbnHelp}>
          <div className={styles.barcodeSample} aria-hidden="true" />
          <p>
            ISBN — 13 цифр под штрих-кодом на задней обложке, например
            <strong> 978-5-389-21133-4</strong>. Дефисы можно не набирать.
          </p>
        </div>
      </div>

      <div className={styles.keypad} aria-label="Цифровая клавиатура">
        {keypad.map((key, index) =>
          key ? (
            <button type="button" key={key} onClick={() => addDigit(key)}>
              {key}
            </button>
          ) : (
            <span key={`empty-${index}`} aria-hidden="true" />
          ),
        )}
        <button
          type="button"
          className={styles.erase}
          onClick={erase}
          disabled={!digits}
        >
          Стереть
        </button>
      </div>
    </div>
  );
}

function CameraErrorScreen({
  failure,
  onManual,
}: {
  failure: CameraFailure;
  onManual: () => void;
}) {
  const denied = failure === "denied";

  return (
    <div className={clsx(styles.screen, styles.paperScreen)}>
      <div className={styles.top}>
        <ScreenHeader />
      </div>
      <div className={styles.failureContent}>
        <div className={styles.cameraOff}>
          <span>{denied ? "камера выключена" : "камеры нет"}</span>
        </div>
        <h1>{denied ? "Камере не дали доступ" : "Камера недоступна"}</h1>
        <p className={styles.failureLead}>
          {denied
            ? "Ничего страшного: ISBN с задней обложки можно ввести руками — это те же полминуты. Камеру можно включить позже, она не обязательна."
            : "На этом устройстве не получилось открыть камеру. ISBN с задней обложки всё равно можно ввести руками."}
        </p>
        {denied && (
          <div className={styles.recovery}>
            <div className={styles.eyebrow}>Если захочешь включить</div>
            <ol>
              <li>Замок рядом с адресом сайта</li>
              <li>«Разрешения» → «Камера»</li>
              <li>Обновить страницу</li>
            </ol>
          </div>
        )}
      </div>
      <footer className={styles.bottomAction}>
        <Button onClick={onManual}>Ввести ISBN вручную</Button>
      </footer>
    </div>
  );
}

function PendingScreen() {
  return (
    <div className={clsx(styles.screen, styles.paperScreen)}>
      <div className={styles.top}>
        <ScreenHeader closeDisabled />
      </div>
      <div className={styles.pending} role="status" aria-live="polite">
        <div className={styles.pendingMark} aria-hidden="true">
          <span />
          <span />
          <span />
        </div>
        <h1>Записываем книгу…</h1>
        <p>Это займёт пару секунд.</p>
      </div>
    </div>
  );
}

function SuccessScreen({
  result,
  onDone,
}: {
  result: BorrowResponse;
  onDone: () => void;
}) {
  const level = bookLevel(result.book.level);

  return (
    <div className={clsx(styles.screen, styles.paperScreen)}>
      <div className={styles.successContent} aria-live="polite">
        <div className={styles.successEyebrow}>Записано на тебя</div>
        <h1>
          Книга у тебя
          <br />
          до {formatDueDate(result.due_at)}
        </h1>

        <div className={styles.successBook}>
          <div className={styles.successCover}>
            <BookCover book={result.book} />
          </div>
          <div className={styles.successBookInfo}>
            <div
              className={clsx(
                styles.successLevel,
                styles[`${result.book.level}Text`],
              )}
            >
              <LevelBars level={result.book.level} />
              <span>{level.label}</span>
            </div>
            <h2>{result.book.title}</h2>
            <p>{result.book.author}</p>
            <div className={styles.successPages}>
              <strong>{result.book.page_count}</strong>
              <span>{pageWord(result.book.page_count)}</span>
            </div>
          </div>
        </div>

        <div className={styles.loanSlip}>
          <div className={styles.eyebrow}>Формуляр</div>
          <dl>
            <div>
              <dt>Взято</dt>
              <dd>{formatDueDate(result.taken_at)}</dd>
            </div>
            <div>
              <dt>Вернуть</dt>
              <dd>{formatDueDate(result.due_at)}</dd>
            </div>
          </dl>
        </div>
      </div>
      <footer className={styles.bottomAction}>
        <Button onClick={onDone}>Готово</Button>
      </footer>
    </div>
  );
}

function BorrowErrorScreen({
  attempt,
  onRetry,
  onManual,
  onCamera,
}: {
  attempt: Extract<BorrowAttempt, { status: "error" }>;
  onRetry: () => void;
  onManual: () => void;
  onCamera: () => void;
}) {
  const { error, source } = attempt;
  const due = error.dueAt ? formatDueDate(error.dueAt) : null;

  let eyebrow = "Не получилось взять книгу";
  let title = "Попробуй ещё раз";
  let lead = error.message;
  let actions = (
    <>
      <Button onClick={onRetry}>Повторить</Button>
      <Button
        variant="secondary"
        onClick={source === "camera" ? onManual : onCamera}
      >
        {source === "camera" ? "Ввести ISBN" : "Сканировать камерой"}
      </Button>
    </>
  );

  if (error.code === "book_not_found" || error.code === "invalid_isbn") {
    eyebrow = "Штрих-код не найден";
    title = "Такого кода нет в каталоге";
    lead =
      error.code === "invalid_isbn"
        ? "Проверь 13 цифр ISBN под штрих-кодом на задней обложке."
        : "Возможно, книгу ещё не добавили. Проверь ISBN или попробуй отсканировать ещё раз.";
    actions = (
      <>
        <Button onClick={onManual}>
          {source === "manual" ? "Исправить ISBN" : "Ввести ISBN"}
        </Button>
        <Button variant="secondary" onClick={onCamera}>
          Ещё раз камерой
        </Button>
      </>
    );
  } else if (error.code === "book_unavailable") {
    eyebrow = "Книгу уже взяли";
    title = error.book
      ? `${error.book.title} у ${error.borrowerName ?? "другого читателя"}${due ? ` — до ${due}` : ""}`
      : "Эту книгу уже взяли";
    lead = error.borrowerName
      ? `Спроси у ${error.borrowerName}, дочитана ли книга: возможно, её вернут раньше.`
      : "Выбери другую книгу на полке.";
    actions = <ButtonLink href="/">Выбрать другую книгу</ButtonLink>;
  } else if (error.code === "loan_limit") {
    eyebrow = "У тебя уже есть книга";
    title = error.book
      ? `Сначала верни ${error.book.title}`
      : "Сначала верни книгу";
    lead = `Правило клуба: одна книга на руках.${due ? ` Срок — до ${due}, вернуть можно в любой день.` : ""}`;
    actions = <ButtonLink href="/">Вернуться в каталог</ButtonLink>;
  } else if (error.code === "book_lost") {
    eyebrow = "Книга не выдаётся";
    title = error.book
      ? `${error.book.title} отмечена как потерянная`
      : "Книга отмечена как потерянная";
    lead = "Обратись к учителю: возможно, статус книги нужно исправить.";
    actions = <ButtonLink href="/">Вернуться в каталог</ButtonLink>;
  } else if (error.status === 0) {
    eyebrow = "Нет связи с сервером";
    title = "Не удалось проверить выдачу";
    lead =
      "ISBN сохранён. Проверь интернет и повтори запрос — если книга уже записалась, появится то же подтверждение.";
  } else if (error.status === 401) {
    eyebrow = "Сессия закончилась";
    title = "Нужно войти заново";
    lead = "Обнови страницу и войди, затем повтори сканирование.";
    actions = (
      <Button onClick={() => window.location.reload()}>
        Обновить страницу
      </Button>
    );
  }

  return (
    <div className={clsx(styles.screen, styles.paperScreen)}>
      <div className={styles.top}>
        <ScreenHeader />
      </div>
      <div className={styles.borrowError} role="alert">
        <div className={styles.eyebrow}>{eyebrow}</div>
        <div className={styles.errorRule}>
          <h1>{title}</h1>
          <p>{lead}</p>
        </div>
      </div>
      <footer className={styles.resultActions}>{actions}</footer>
    </div>
  );
}

export function Scan() {
  const navigate = useNavigate();
  const [mode, setMode] = useState<ScanMode>(() =>
    isWideViewport() ? "manual" : "camera",
  );
  const [failure, setFailure] = useState<CameraFailure | null>(null);
  const [attempt, setAttempt] = useState<BorrowAttempt | null>(null);
  const [manualInitial, setManualInitial] = useState("");
  const submissionLocked = useRef(false);

  const submitISBN = useCallback((isbn: string, source: ScanMode) => {
    if (submissionLocked.current) return;
    submissionLocked.current = true;
    setAttempt({ status: "pending", isbn, source });

    api<BorrowResponse>("/loans", { method: "POST", body: { isbn } }).then(
      (result) => setAttempt({ status: "success", result }),
      (error: unknown) => {
        const apiError =
          error instanceof ApiError
            ? error
            : new ApiError(0, "Не получилось связаться с сервером.");
        setAttempt({ status: "error", isbn, source, error: apiError });
      },
    );
  }, []);

  const showCamera = useCallback(() => {
    setFailure(null);
    setAttempt(null);
    setManualInitial("");
    submissionLocked.current = false;
    setMode("camera");
  }, []);

  const showManual = useCallback(() => {
    setFailure(null);
    setAttempt(null);
    setManualInitial("");
    submissionLocked.current = false;
    setMode("manual");
  }, []);

  const cameraDetected = useCallback(
    (value: string) => {
      submitISBN(value, "camera");
    },
    [submitISBN],
  );

  const manualAccepted = useCallback(
    (value: string) => {
      submitISBN(value, "manual");
    },
    [submitISBN],
  );

  const cameraFailed = useCallback((nextFailure: CameraFailure) => {
    setFailure(nextFailure);
  }, []);

  const retryBorrow = useCallback(() => {
    if (!attempt || attempt.status !== "error") return;
    const { isbn, source } = attempt;
    submissionLocked.current = false;
    submitISBN(isbn, source);
  }, [attempt, submitISBN]);

  const editManualISBN = useCallback(() => {
    const isbn = attempt && attempt.status === "error" ? attempt.isbn : "";
    setManualInitial(isbn);
    setAttempt(null);
    setFailure(null);
    submissionLocked.current = false;
    setMode("manual");
  }, [attempt]);

  if (attempt?.status === "pending") {
    return <PendingScreen />;
  }

  if (attempt?.status === "success") {
    return (
      <SuccessScreen
        result={attempt.result}
        onDone={() => navigate("/", { replace: true })}
      />
    );
  }

  if (attempt?.status === "error") {
    return (
      <BorrowErrorScreen
        attempt={attempt}
        onRetry={retryBorrow}
        onManual={editManualISBN}
        onCamera={showCamera}
      />
    );
  }

  if (mode === "camera" && failure) {
    return <CameraErrorScreen failure={failure} onManual={showManual} />;
  }

  if (mode === "manual") {
    return (
      <ManualScreen
        initialValue={manualInitial}
        onCamera={showCamera}
        onAccepted={manualAccepted}
      />
    );
  }

  return (
    <CameraScreen
      onManual={showManual}
      onDetected={cameraDetected}
      onFailure={cameraFailed}
    />
  );
}
