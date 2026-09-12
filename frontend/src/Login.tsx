import { useState, type FormEvent } from "react";
import { api, ApiError } from "./api.ts";
import styles from "./Login.module.scss";

type Props = {
  onSignedIn: (token: string) => void;
  initialError: string | null;
};

// A bare login, enough to test both auth paths. The real screen is PRD §9.1.
export function Login({ onSignedIn, initialError }: Props) {
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  // null until the code is checked, then whether the account has a password:
  // one screen for the first login and every one after it (PRD §8).
  const [hasPassword, setHasPassword] = useState<boolean | null>(null);
  const [error, setError] = useState(initialError);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError(null);
    setBusy(true);
    try {
      if (hasPassword === null) {
        const res = await api<{ has_password: boolean }>("/auth/code/check", {
          method: "POST",
          body: { code },
        });
        setHasPassword(res.has_password);
      } else {
        const res = await api<{ token: string }>("/auth/code/login", {
          method: "POST",
          body: { code, password },
        });
        onSignedIn(res.token);
      }
    } catch (err) {
      if (!(err instanceof ApiError)) throw err;
      setError(err.message);
      // Someone set the password from another device in the meantime.
      if (err.status === 409) setHasPassword(true);
    } finally {
      setBusy(false);
    }
  }

  function changeCode() {
    setHasPassword(null);
    setPassword("");
    setError(null);
  }

  return (
    <div className={styles.login}>
      <a className={styles.button} href="/api/auth/yandex">
        Войти через Яндекс
      </a>
      <a className={styles.button} href="/api/auth/vk">
        Войти через VK ID
      </a>

      <form className={styles.form} onSubmit={submit}>
        <label className={styles.field}>
          Код ученика
          <input
            value={code}
            onChange={(e) => setCode(e.target.value)}
            readOnly={hasPassword !== null}
            autoCapitalize="characters"
            autoComplete="username"
            spellCheck={false}
            required
          />
        </label>
        {hasPassword !== null && (
          <label className={styles.field}>
            {hasPassword ? "Введи пароль" : "Придумай пароль"}
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete={hasPassword ? "current-password" : "new-password"}
              minLength={hasPassword ? undefined : 6}
              autoFocus
              required
            />
            {!hasPassword && (
              <small>
                Не короче 6 символов. Если забудешь, учитель его сбросит.
              </small>
            )}
          </label>
        )}
        {error && (
          <p className={styles.error} role="alert">
            {error}
          </p>
        )}
        <button className={styles.button} type="submit" disabled={busy}>
          {hasPassword === null ? "Дальше" : "Войти"}
        </button>
        {hasPassword !== null && (
          <button type="button" onClick={changeCode}>
            Другой код
          </button>
        )}
      </form>
    </div>
  );
}
