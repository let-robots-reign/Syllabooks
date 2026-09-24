import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, ApiError } from "../api.ts";
import { Button } from "../ui/Button.tsx";
import { Modal } from "../ui/Modal.tsx";
import { TextField } from "../ui/TextField.tsx";
import styles from "./BookRequest.module.scss";

// Matches the server's limit on a requested title.
const maxTitleLength = 200;

// BookRequest lets a student suggest a book for the teacher to buy. Students
// never see requests again, so a sent request ends with a thank-you only.
export function BookRequest() {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const doneRef = useRef<HTMLButtonElement>(null);

  // The field that had focus is gone once the thank-you replaces the form.
  useEffect(() => {
    if (sent) doneRef.current?.focus();
  }, [sent]);

  const start = () => {
    setTitle("");
    setSent(false);
    setError(null);
    setOpen(true);
  };

  const close = () => {
    if (!sending) setOpen(false);
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const trimmed = title.trim();
    if (!trimmed || sending) return;
    setSending(true);
    setError(null);
    try {
      await api("/book-requests", { method: "POST", body: { title: trimmed } });
      setSent(true);
    } catch (err) {
      setError((err as ApiError).message);
    } finally {
      setSending(false);
    }
  };

  return (
    <div className={styles.prompt}>
      <p>Не нашлось желаемой книги? Напиши, что тебе бы хотелось прочитать!</p>
      <Button variant="secondary" compact onClick={start}>
        Предложить книгу
      </Button>

      <Modal
        open={open}
        onClose={close}
        title="Предложить книгу"
        initialFocusRef={inputRef}
      >
        {sent ? (
          <div className={styles.thanks} role="status">
            <svg
              className={styles.check}
              viewBox="0 0 88 88"
              aria-hidden="true"
            >
              <circle className={styles.checkCircle} cx="44" cy="44" r="44" />
              <path className={styles.checkMark} d="M26 45.5l12 12 24-26" />
            </svg>
            <p>Спасибо! Постараемся приобрести эту книгу для тебя!</p>
            <Button ref={doneRef} onClick={close}>
              Закрыть
            </Button>
          </div>
        ) : (
          <form className={styles.form} onSubmit={submit}>
            <TextField
              ref={inputRef}
              label="Название книги"
              value={title}
              maxLength={maxTitleLength}
              autoComplete="off"
              enterKeyHint="send"
              onChange={(event) => setTitle(event.target.value)}
              error={error}
              hint="Можно по-английски или по-русски, как удобно"
            />
            <div className={styles.actions}>
              <Button type="submit" disabled={!title.trim() || sending}>
                {sending ? "Отправляем…" : "Отправить"}
              </Button>
              <Button
                variant="quiet"
                compact
                onClick={close}
                disabled={sending}
              >
                Отмена
              </Button>
            </div>
          </form>
        )}
      </Modal>
    </div>
  );
}
