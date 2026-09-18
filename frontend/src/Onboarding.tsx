import { useState } from "react";
import clsx from "clsx";
import { api, ApiError, type BookLevel } from "./api.ts";
import { LevelBars } from "./catalog/BookVisuals.tsx";
import { Button } from "./ui/Button.tsx";
import { Eyebrow } from "./ui/Eyebrow.tsx";
import styles from "./Onboarding.module.scss";

const levels: Array<{
  level: BookLevel;
  label: string;
  description: string;
}> = [
  {
    level: "green",
    label: "Просто",
    description: "короткие, адаптированные — с них начинают все",
  },
  {
    level: "yellow",
    label: "Средняя сложность",
    description: "настоящие романы простым языком",
  },
  {
    level: "red",
    label: "Сложно",
    description: "длинные или старомодные — вызов, не запрет",
  },
];

export function Onboarding({ onComplete }: { onComplete: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const complete = async () => {
    setBusy(true);
    setError(null);
    try {
      await api<void>("/onboarding/complete", { method: "POST" });
      onComplete();
    } catch (err) {
      if (!(err instanceof ApiError)) throw err;
      setError(err.message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className={styles.screen} aria-labelledby="onboarding-title">
      <div className={styles.content}>
        <h1 className={styles.title} id="onboarding-title">
          Как это работает
        </h1>
        <p className={styles.intro}>
          Тридцать секунд, и больше не понадобится.
        </p>

        <section className={styles.levels}>
          <Eyebrow as="h2">Три уровня сложности</Eyebrow>
          <ul className={styles.levelList}>
            {levels.map(({ level, label, description }) => (
              <li className={styles.level} key={level}>
                <LevelBars level={level} />
                <div>
                  <h2 className={clsx(styles.levelName, styles[level])}>
                    {label}
                  </h2>
                  <p className={styles.levelDescription}>{description}</p>
                </div>
              </li>
            ))}
          </ul>
          <p className={styles.levelNote}>
            Цвет корешка на обложке = уровень. Слово всегда рядом, в фильтре и
            на странице книги.
          </p>
        </section>

        <div className={styles.rules}>
          <section className={styles.rule}>
            <strong className={styles.number}>1</strong>
            <div>
              <h2 className={styles.ruleTitle}>книга на руках</h2>
              <p className={styles.ruleText}>
                Взял новую — сначала верни прежнюю.
              </p>
            </div>
          </section>
          <section className={styles.rule}>
            <strong className={styles.number}>21</strong>
            <div>
              <h2 className={styles.ruleTitle}>день на чтение</h2>
              <p className={styles.ruleText}>
                Не успеваешь — ничего страшного. Попроси учителя продлить.
              </p>
            </div>
          </section>
          <section className={styles.rule}>
            <strong className={styles.number}>2</strong>
            <div>
              <h2 className={styles.ruleTitle}>кода при возврате</h2>
              <p className={styles.ruleText}>
                Сначала код на шкафу, потом сама книга. Не сканируется — можно
                вручную.
              </p>
            </div>
          </section>
        </div>
      </div>

      <footer className={styles.footer}>
        {error && (
          <p className={styles.error} role="alert">
            {error}
          </p>
        )}
        <Button onClick={complete} disabled={busy}>
          {busy
            ? "Сохраняем…"
            : error
              ? "Попробовать ещё раз"
              : "Понятно, к полке"}
        </Button>
      </footer>
    </section>
  );
}
