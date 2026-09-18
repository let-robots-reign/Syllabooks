import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type FormEvent,
  type KeyboardEvent,
} from "react";
import clsx from "clsx";
import { useNavigate, useParams } from "react-router-dom";
import {
  api,
  ApiError,
  type Book,
  type BookInput,
  type BookLookup,
} from "../api.ts";
import { type CameraFailure } from "../cameraRecovery.ts";
import { Link } from "../ui/Link.tsx";
import { bookLevels } from "../bookLevels.ts";
import { CameraViewport } from "../scan/Scan.tsx";
import styles from "./AdminBookForm.module.scss";

const blankBook: BookInput = {
  isbn: "",
  title: "",
  author: "",
  level: "green",
  page_count: 0,
  description: "",
};

export function AdminBookForm() {
  const { id } = useParams();
  const navigate = useNavigate();
  const isNew = id === undefined;
  const isbnInput = useRef<HTMLInputElement>(null);
  const formElement = useRef<HTMLFormElement>(null);
  const lookupRequest = useRef(0);
  const [book, setBook] = useState<Book | null>(null);
  const [form, setForm] = useState<BookInput>(blankBook);
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [lookingUp, setLookingUp] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [lookupSource, setLookupSource] = useState<string | null>(null);
  const [lookupCover, setLookupCover] = useState<string | null>(null);
  const [coverFile, setCoverFile] = useState<File | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [scanAttempt, setScanAttempt] = useState(0);
  const [cameraFailure, setCameraFailure] = useState<CameraFailure | null>(
    null,
  );
  const [scanUnreadable, setScanUnreadable] = useState(false);

  useEffect(() => {
    if (isNew) {
      isbnInput.current?.focus();
      return;
    }
    let current = true;
    api<Book>(`/admin/books/${id}`).then(
      (result) => {
        if (!current) return;
        setBook(result);
        setForm({
          isbn: result.isbn ?? "",
          title: result.title,
          author: result.author,
          level: result.level,
          page_count: result.page_count,
          description: result.description ?? "",
        });
        setLoading(false);
      },
      (err: ApiError) => {
        if (!current) return;
        setError(err.message);
        setLoading(false);
      },
    );
    return () => {
      current = false;
    };
  }, [id, isNew]);

  const localPreview = useMemo(() => {
    return coverFile ? URL.createObjectURL(coverFile) : null;
  }, [coverFile]);

  useEffect(() => {
    return () => {
      if (localPreview) URL.revokeObjectURL(localPreview);
    };
  }, [localPreview]);

  useEffect(() => {
    if (!scanning) return;

    const closeOnEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") setScanning(false);
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [scanning]);

  const originalISBN = book?.isbn ?? "";
  const cover =
    localPreview ??
    lookupCover ??
    (form.isbn === originalISBN ? book?.cover_url : null);
  const descriptionLength = Array.from(form.description).length;

  const update = <K extends keyof BookInput>(key: K, value: BookInput[K]) => {
    setForm((current) => ({ ...current, [key]: value }));
    setError(null);
  };

  const changeISBN = (value: string) => {
    update("isbn", value);
    setLookupCover(null);
    setLookupSource(null);
    setLookupError(null);
  };

  const openScanner = () => {
    setCameraFailure(null);
    setScanUnreadable(false);
    setScanAttempt((attempt) => attempt + 1);
    setScanning(true);
  };

  const scanFailed = useCallback((failure: CameraFailure) => {
    setCameraFailure(failure);
  }, []);

  const lookup = useCallback(
    async (isbn = form.isbn, replace = false) => {
      if (!isbn.trim() || (lookingUp && !replace)) return;
      const request = ++lookupRequest.current;
      setLookingUp(true);
      setLookupError(null);
      try {
        const result = await api<BookLookup>("/admin/books/lookup", {
          method: "POST",
          body: { isbn },
        });
        if (request !== lookupRequest.current) return;
        setForm((current) => ({
          ...current,
          isbn: result.isbn,
          title: replace ? result.title : current.title.trim() || result.title,
          author: replace
            ? result.author
            : current.author.trim() || result.author,
          page_count: replace
            ? result.page_count
            : current.page_count || result.page_count,
          description: replace
            ? result.description
            : current.description.trim() || result.description,
        }));
        setLookupCover(result.cover_preview);
        setLookupSource(sourceLabel(result.source));
      } catch (err) {
        if (request === lookupRequest.current) {
          setLookupError((err as ApiError).message);
        }
      } finally {
        if (request === lookupRequest.current) setLookingUp(false);
      }
    },
    [form.isbn, lookingUp],
  );

  const scannedISBN = useCallback(
    (value: string) => {
      setForm((current) =>
        current.isbn === value
          ? current
          : {
              ...current,
              isbn: value,
              title: "",
              author: "",
              page_count: 0,
              description: "",
            },
      );
      setLookupCover(null);
      setLookupSource(null);
      setLookupError(null);
      setCameraFailure(null);
      setScanUnreadable(false);
      setScanning(false);
      void lookup(value, true);
    },
    [lookup],
  );

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (saving) return;
    setSaving(true);
    setError(null);
    try {
      const saved = await api<Book>(
        isNew ? "/admin/books" : `/admin/books/${id}`,
        {
          method: isNew ? "POST" : "PUT",
          body: form,
        },
      );
      setBook(saved);

      let upload = coverFile;
      if (!upload && !isNew && lookupCover) {
        upload = await dataURLFile(lookupCover);
      }
      if (upload) {
        const data = new FormData();
        data.set("cover", upload);
        try {
          const withCover = await api<Book>(`/admin/books/${saved.id}/cover`, {
            method: "PUT",
            body: data,
          });
          setBook(withCover);
        } catch (err) {
          if (isNew) navigate(`/admin/books/${saved.id}`, { replace: true });
          setError(
            `Книга сохранена, но обложка не загрузилась. ${(err as ApiError).message}`,
          );
          setSaving(false);
          return;
        }
      }
      navigate("/admin/books");
    } catch (err) {
      setError((err as ApiError).message);
      setSaving(false);
    }
  };

  const removeCover = async () => {
    if (coverFile || lookupCover) {
      setCoverFile(null);
      setLookupCover(null);
      return;
    }
    if (!book?.cover_url) return;
    setError(null);
    try {
      const updated = await api<Book>(`/admin/books/${book.id}/cover`, {
        method: "DELETE",
      });
      setBook(updated);
    } catch (err) {
      setError((err as ApiError).message);
    }
  };

  const removeBook = async () => {
    if (!book) return;
    setSaving(true);
    setError(null);
    try {
      await api(`/admin/books/${book.id}`, { method: "DELETE" });
      navigate("/admin/books");
    } catch (err) {
      setError((err as ApiError).message);
      setSaving(false);
      setConfirmDelete(false);
    }
  };

  const handleKeys = (event: KeyboardEvent<HTMLFormElement>) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      formElement.current?.requestSubmit();
    }
  };

  if (loading) return <p className={styles.state}>Загрузка книги…</p>;

  return (
    <section className={styles.page}>
      <div className={styles.toolbar}>
        <Link href="/admin/books" className={styles.back}>
          ← Каталог
        </Link>
        <div className={styles.toolbarActions}>
          {!isNew && !confirmDelete && (
            <button
              className={styles.delete}
              onClick={() => setConfirmDelete(true)}
            >
              Удалить из каталога
            </button>
          )}
          {confirmDelete && (
            <div className={styles.confirmDelete}>
              <span>Удалить навсегда?</span>
              <button onClick={removeBook} disabled={saving}>
                Да, удалить
              </button>
              <button onClick={() => setConfirmDelete(false)}>Нет</button>
            </div>
          )}
          <Link href="/admin/books" className={styles.cancel}>
            Отменить
          </Link>
          <button
            className={styles.save}
            type="submit"
            form="book-form"
            disabled={saving}
          >
            {saving ? "Сохраняем…" : "Сохранить"}
          </button>
        </div>
      </div>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      <form
        id="book-form"
        ref={formElement}
        className={styles.form}
        onSubmit={submit}
        onKeyDown={handleKeys}
      >
        <aside className={styles.coverColumn}>
          <span className={styles.eyebrow}>Обложка</span>
          <div className={styles.cover}>
            {cover ? (
              <img src={cover} alt="Обложка книги" />
            ) : (
              <div className={styles.coverPlaceholder}>нет обложки</div>
            )}
            <span className={clsx(styles.coverSpine, styles[form.level])} />
          </div>
          <label className={styles.coverButton}>
            {cover ? "Заменить обложку" : "Загрузить обложку"}
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp"
              onChange={(event: ChangeEvent<HTMLInputElement>) => {
                setCoverFile(event.target.files?.[0] ?? null);
                setLookupCover(null);
                event.target.value = "";
              }}
            />
          </label>
          {cover && (
            <button
              type="button"
              className={styles.removeCover}
              onClick={removeCover}
            >
              Убрать обложку
            </button>
          )}
          <p className={styles.coverHint}>
            Только передняя обложка. JPEG, PNG или WebP, до 8 МБ.
          </p>
        </aside>

        <div className={styles.fields}>
          <div className={styles.isbnRow}>
            <Field label="ISBN" className={styles.isbnField}>
              <input
                ref={isbnInput}
                inputMode="numeric"
                value={form.isbn}
                placeholder="978-0-380-80734-5"
                onChange={(event) => changeISBN(event.target.value)}
                onKeyDown={(event) => {
                  if (
                    event.key === "Enter" &&
                    !event.metaKey &&
                    !event.ctrlKey
                  ) {
                    event.preventDefault();
                    void lookup();
                  }
                }}
              />
            </Field>
            <button type="button" className={styles.scan} onClick={openScanner}>
              Сканировать
            </button>
            <button
              type="button"
              className={styles.lookup}
              disabled={!form.isbn.trim() || lookingUp}
              onClick={() => void lookup()}
            >
              {lookingUp ? "Ищем…" : "Найти по ISBN"}
            </button>
          </div>
          {(lookupError || lookupSource) && (
            <p
              className={
                lookupError ? styles.lookupError : styles.lookupSuccess
              }
            >
              {lookupError ?? `Данные найдены: ${lookupSource}`}
            </p>
          )}

          <div className={styles.twoColumns}>
            <Field label="Название">
              <input
                required
                value={form.title}
                onChange={(event) => update("title", event.target.value)}
              />
            </Field>
            <Field label="Автор">
              <input
                required
                value={form.author}
                onChange={(event) => update("author", event.target.value)}
              />
            </Field>
          </div>

          <fieldset className={styles.levelField}>
            <legend className={styles.eyebrow}>Уровень</legend>
            <div className={styles.levels}>
              {bookLevels.map((level, index) => (
                <button
                  key={level.value}
                  type="button"
                  role="radio"
                  aria-checked={form.level === level.value}
                  className={clsx(
                    styles.levelCard,
                    styles[level.value],
                    form.level === level.value && styles.selected,
                  )}
                  onClick={() => update("level", level.value)}
                >
                  <span className={styles.levelBars} aria-hidden="true">
                    {[0, 1, 2].map((bar) => (
                      <span
                        key={bar}
                        className={bar <= index ? styles.filled : undefined}
                      />
                    ))}
                  </span>
                  <strong>{level.label}</strong>
                  <small>
                    {form.level === level.value ? "выбрано" : level.hint}
                  </small>
                </button>
              ))}
            </div>
          </fieldset>

          <div className={styles.pageCountRow}>
            <Field label="Страниц">
              <input
                required
                min="1"
                type="number"
                value={form.page_count || ""}
                onChange={(event) =>
                  update("page_count", Number(event.target.value))
                }
              />
            </Field>
          </div>

          <label className={styles.descriptionField}>
            <span className={styles.descriptionLabel}>
              <span className={styles.eyebrow}>Описание для ученика</span>
              <span className={clsx(descriptionLength > 240 && styles.tooLong)}>
                {descriptionLength} / 240 знаков
              </span>
            </span>
            <textarea
              maxLength={240}
              value={form.description}
              onChange={(event) => update("description", event.target.value)}
            />
          </label>
          <p className={styles.shortcut}>⌘/Ctrl + Enter — сохранить</p>
        </div>
      </form>

      {scanning && (
        <div
          className={styles.scannerOverlay}
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setScanning(false);
          }}
        >
          <section
            className={styles.scannerDialog}
            role="dialog"
            aria-modal="true"
            aria-labelledby="scanner-title"
          >
            <header className={styles.scannerHeader}>
              <div>
                <span className={styles.eyebrow}>ISBN книги</span>
                <h2 id="scanner-title">Сканировать штрих-код</h2>
              </div>
              <button
                type="button"
                className={styles.closeScanner}
                aria-label="Закрыть сканер"
                onClick={() => setScanning(false)}
              >
                ×
              </button>
            </header>

            {cameraFailure || scanUnreadable ? (
              <div className={styles.cameraFailure} role="alert">
                <strong>
                  {scanUnreadable
                    ? "Штрих-код не распознан"
                    : cameraFailure === "denied"
                      ? "Нет доступа к камере"
                      : "Камера недоступна"}
                </strong>
                <p>
                  {scanUnreadable
                    ? "Добавьте света, протрите камеру и попробуйте ещё раз — или введите ISBN вручную."
                    : cameraFailure === "denied"
                      ? "Разрешите сайту использовать камеру в настройках браузера или введите ISBN вручную."
                      : "На этом устройстве не получилось открыть камеру. ISBN можно ввести вручную."}
                </p>
                <div className={styles.cameraFailureActions}>
                  <button
                    type="button"
                    onClick={() => {
                      setCameraFailure(null);
                      setScanUnreadable(false);
                      setScanAttempt((attempt) => attempt + 1);
                    }}
                  >
                    Попробовать снова
                  </button>
                  <button type="button" onClick={() => setScanning(false)}>
                    Ввести ISBN вручную
                  </button>
                </div>
              </div>
            ) : (
              <div className={styles.scannerViewport}>
                <CameraViewport
                  key={scanAttempt}
                  onDetected={scannedISBN}
                  onFailure={scanFailed}
                  onUnreadable={() => setScanUnreadable(true)}
                />
              </div>
            )}

            <p className={styles.scannerHint}>
              Наведите камеру на штрих-код ISBN на задней обложке. После
              распознавания код появится в поле формы.
            </p>
          </section>
        </div>
      )}
    </section>
  );
}

function Field({
  label,
  className,
  children,
}: {
  label: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <label className={clsx(styles.field, className)}>
      <span className={styles.eyebrow}>{label}</span>
      {children}
    </label>
  );
}

const sourceLabel = (source: BookLookup["source"]) => {
  switch (source) {
    case "openlibrary":
      return "Open Library";
    case "google":
      return "Google Books";
    default:
      return "Open Library + Google Books";
  }
};

const dataURLFile = async (dataURL: string) => {
  const response = await fetch(dataURL);
  const blob = await response.blob();
  const extension =
    blob.type === "image/png"
      ? "png"
      : blob.type === "image/webp"
        ? "webp"
        : "jpg";
  return new File([blob], `cover.${extension}`, { type: blob.type });
};
