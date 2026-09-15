import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
} from "react";
import type { QuaggaJSResultObject, QuaggaJSStatic } from "@ericblade/quagga2";
import clsx from "clsx";
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
type ScanResult = { value: string; source: ScanMode };

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

function ScreenHeader() {
  return (
    <header className={styles.header}>
      <div className={styles.screenTitle}>Взять книгу</div>
      <Link href="/" className={styles.close} aria-label="Закрыть сканирование">
        ×
      </Link>
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
}: {
  onCamera: () => void;
  onAccepted: (value: string) => void;
}) {
  const [digits, setDigits] = useState("");
  const error = manualIsbnError(digits);

  useEffect(() => {
    if (digits.length === 13 && !error) onAccepted(digits);
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

function ResultScreen({
  result,
  onAgain,
}: {
  result: ScanResult;
  onAgain: () => void;
}) {
  return (
    <div className={clsx(styles.screen, styles.paperScreen)}>
      <div className={styles.top}>
        <ScreenHeader />
      </div>
      <div className={styles.resultContent} aria-live="polite">
        <div className={styles.eyebrow}>
          {result.source === "camera" ? "Штрих-код распознан" : "ISBN введён"}
        </div>
        <h1>EAN-13 найден</h1>
        <div className={styles.resultCode}>{formatIsbn(result.value)}</div>
        <p>
          Результат этого технического теста. Книга пока не записана на тебя.
        </p>
      </div>
      <footer className={styles.resultActions}>
        <Button onClick={onAgain}>Ещё раз</Button>
        <ButtonLink href="/" variant="quiet">
          Закрыть
        </ButtonLink>
      </footer>
    </div>
  );
}

export function Scan() {
  const [mode, setMode] = useState<ScanMode>(() =>
    isWideViewport() ? "manual" : "camera",
  );
  const [failure, setFailure] = useState<CameraFailure | null>(null);
  const [result, setResult] = useState<ScanResult | null>(null);

  const showCamera = useCallback(() => {
    setFailure(null);
    setResult(null);
    setMode("camera");
  }, []);

  const showManual = useCallback(() => {
    setFailure(null);
    setResult(null);
    setMode("manual");
  }, []);

  const cameraDetected = useCallback((value: string) => {
    setResult({ value, source: "camera" });
  }, []);

  const manualAccepted = useCallback((value: string) => {
    setResult({ value, source: "manual" });
  }, []);

  const cameraFailed = useCallback((nextFailure: CameraFailure) => {
    setFailure(nextFailure);
  }, []);

  const tryAgain = useCallback(() => {
    setResult(null);
    setFailure(null);
  }, []);

  if (result) {
    return <ResultScreen result={result} onAgain={tryAgain} />;
  }

  if (mode === "camera" && failure) {
    return <CameraErrorScreen failure={failure} onManual={showManual} />;
  }

  if (mode === "manual") {
    return <ManualScreen onCamera={showCamera} onAccepted={manualAccepted} />;
  }

  return (
    <CameraScreen
      onManual={showManual}
      onDetected={cameraDetected}
      onFailure={cameraFailed}
    />
  );
}
