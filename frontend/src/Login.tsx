import { useState, type FormEvent } from "react";
import { api, ApiError } from "./api.ts";
import styles from "./Login.module.scss";
import { Button, ButtonLink } from "./ui/Button.tsx";
import { cx } from "./ui/cx.ts";
import { Eyebrow } from "./ui/Eyebrow.tsx";
import { Link } from "./ui/Link.tsx";
import { TextField } from "./ui/TextField.tsx";
import { Wordmark } from "./ui/Wordmark.tsx";

type Props = {
  onSignedIn: (token: string) => void;
  initialError: string | null;
};

// The sign-in screens, design 2g and 4a–4d. "start" leads with Yandex and VK
// and folds the student code into one row; "code" unfolds it in place. Then
// the code branches as in PRD §8: "password" asks to make one up or to enter
// it, and "notFound" explains the miss.
type Step = "start" | "code" | "password" | "notFound";

export function Login({ onSignedIn, initialError }: Props) {
  const [step, setStep] = useState<Step>("start");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [hasPassword, setHasPassword] = useState(false);
  const [passwordShown, setPasswordShown] = useState(false);
  const [forgotShown, setForgotShown] = useState(false);
  // notice is a failed Yandex or VK login; error belongs to the code steps.
  const [notice, setNotice] = useState(initialError);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const shownCode = code.trim().toUpperCase();

  function goTo(next: Step) {
    setError(null);
    setStep(next);
  }

  async function checkCode(event: FormEvent) {
    event.preventDefault();
    setNotice(null);
    setError(null);
    setBusy(true);
    try {
      const res = await api<{ has_password: boolean }>("/auth/code/check", {
        method: "POST",
        body: { code },
      });
      setHasPassword(res.has_password);
      setPassword("");
      setStep("password");
    } catch (err) {
      if (!(err instanceof ApiError)) throw err;
      if (err.status === 404) setStep("notFound");
      else setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  async function logIn(event: FormEvent) {
    event.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const res = await api<{ token: string }>("/auth/code/login", {
        method: "POST",
        body: { code, password },
      });
      onSignedIn(res.token);
    } catch (err) {
      if (!(err instanceof ApiError)) throw err;
      setError(err.message);
      // Someone set the password from another device in the meantime.
      if (err.status === 409) setHasPassword(true);
    } finally {
      setBusy(false);
    }
  }

  const intro = (
    <div className={styles.intro}>
      <Wordmark size={44} />
      <p className={styles.pitch}>
        Шкаф у окна в 403л, книги на английском, одна твоя на 21 день.
      </p>
    </div>
  );

  const back = (target: Step) => (
    <button
      type="button"
      className={styles.back}
      aria-label="Назад"
      onClick={() => goTo(target)}
    >
      ←
    </button>
  );

  if (step === "start") {
    return (
      <div className={styles.screen}>
        {intro}
        <div className={styles.bottom}>
          {notice && (
            <p className={styles.notice} role="alert">
              {notice}
            </p>
          )}
          <ButtonLink href="/api/auth/yandex" className={styles.action}>
            <img className={styles.icon} src="/oauth/yandex.svg" alt="" />
            Войти через Яндекс
          </ButtonLink>
          <ButtonLink
            href="/api/auth/vk"
            variant="secondary"
            className={styles.action}
          >
            <img className={styles.icon} src="/oauth/vk.svg" alt="" />
            Войти через VK
          </ButtonLink>
          <button
            type="button"
            className={cx(styles.fold, styles.foldClosed)}
            aria-expanded={false}
            onClick={() => goTo("code")}
          >
            Есть код ученика?
            <span className={styles.sign} aria-hidden="true">
              ＋
            </span>
          </button>
          <p className={styles.privacy}>
            Мы храним только имя и список книг.{" "}
            <Link href="/privacy">Политика конфиденциальности</Link>
          </p>
        </div>
      </div>
    );
  }

  if (step === "code") {
    return (
      <div className={styles.screen}>
        {intro}
        <div className={cx(styles.bottom, styles.unfolded)}>
          <button
            type="button"
            className={cx(styles.fold, styles.foldOpen)}
            aria-expanded={true}
            onClick={() => goTo("start")}
          >
            Код ученика
            <span className={styles.sign} aria-hidden="true">
              −
            </span>
          </button>
          <form className={styles.codeForm} onSubmit={checkCode}>
            <TextField
              label="Код ученика"
              hideLabel
              code
              value={code}
              onChange={(e) => setCode(e.target.value)}
              autoCapitalize="characters"
              autoComplete="username"
              spellCheck={false}
              autoFocus
              required
              hint="Если не хочешь входить через VK или Яндекс, спроси код у учителя"
              error={error}
            />
            <Button type="submit" className={styles.action} disabled={busy}>
              Далее
            </Button>
          </form>
          <div className={styles.links}>
            <a className={styles.link} href="/api/auth/yandex">
              Яндекс
            </a>
            <span className={styles.divider} aria-hidden="true" />
            <a className={styles.link} href="/api/auth/vk">
              VK
            </a>
          </div>
        </div>
      </div>
    );
  }

  if (step === "password") {
    return (
      <form className={styles.screen} onSubmit={logIn}>
        <div>
          {back("code")}
          <div className={styles.body}>
            {/* For password managers: which account this password is for. */}
            <input
              type="text"
              autoComplete="username"
              value={shownCode}
              readOnly
              hidden
            />
            <p className={styles.chip}>
              <span className={styles.chipSpine} />
              <span className={styles.chipCode}>{shownCode}</span>
            </p>
            <h1 className={styles.title}>
              {hasPassword ? "С возвращением" : "Придумай пароль"}
            </h1>
            <p className={styles.lead}>
              {hasPassword
                ? "Введи пароль, который придумал при первом входе."
                : "Первый вход. Пароль нужен, чтобы книги остались за тобой, даже если сменишь телефон."}
            </p>
            <TextField
              className={styles.field}
              label="Пароль"
              hideLabel
              code
              type={passwordShown ? "text" : "password"}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete={hasPassword ? "current-password" : "new-password"}
              minLength={hasPassword ? undefined : 6}
              autoFocus
              required
              trailing={
                <button
                  type="button"
                  className={styles.reveal}
                  onClick={() => setPasswordShown(!passwordShown)}
                >
                  {passwordShown ? "скрыть" : "показать"}
                </button>
              }
              hint={
                hasPassword
                  ? undefined
                  : "От 6 символов. Можно буквы, цифры и точки — без заглавных требований."
              }
              error={error}
            />
            {/* No self-serve reset (PRD §9.1): the teacher resets it. */}
            {hasPassword &&
              !error &&
              (forgotShown ? (
                <p className={styles.forgotText}>
                  Попроси учителя сбросить пароль, а потом войди с тем же кодом
                  и придумай новый.
                </p>
              ) : (
                <button
                  type="button"
                  className={styles.forgot}
                  onClick={() => setForgotShown(true)}
                >
                  Забыл пароль
                </button>
              ))}
          </div>
        </div>
        <div className={styles.bottom}>
          <Button type="submit" className={styles.action} disabled={busy}>
            {hasPassword ? "Войти" : "Сохранить и войти"}
          </Button>
        </div>
      </form>
    );
  }

  return (
    <div className={styles.screen}>
      <div>
        {back("code")}
        <div className={styles.body}>
          <h1 className={styles.title}>Такого кода нет в списке</h1>
          <p className={styles.lead}>
            Проверь, так ли он написан, как на листке. Или спроси код у учителя.
          </p>
          <p className={styles.missing}>
            <span className={styles.missingCode}>{shownCode}</span>
            <span className={styles.missingNote}>не найден</span>
          </p>
          <div className={styles.example}>
            <Eyebrow>Как выглядит код</Eyebrow>
            <div className={styles.exampleRow}>
              <span className={styles.exampleCode}>K7F2MX</span>
              <span className={styles.exampleText}>
                шесть знаков, латинские буквы и цифры
              </span>
            </div>
          </div>
        </div>
      </div>
      <div className={styles.bottom}>
        <Button
          className={styles.action}
          onClick={() => {
            setCode("");
            goTo("code");
          }}
        >
          Ввести другой код
        </Button>
        <Button
          variant="quiet"
          className={styles.quiet}
          onClick={() => goTo("start")}
        >
          Войти через Яндекс или VK
        </Button>
      </div>
    </div>
  );
}
