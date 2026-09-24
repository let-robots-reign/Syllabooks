import { useState, type ReactNode } from "react";
import clsx from "clsx";
import { api, ApiError, type BookLevel } from "./api.ts";
import { LevelBars } from "./catalog/BookVisuals.tsx";
import { Button } from "./ui/Button.tsx";
import { Eyebrow } from "./ui/Eyebrow.tsx";
import {
  BookBarcodeArt,
  DueDateArt,
  PhoneScanArt,
  ShelfArt,
  ShelfCodeArt,
} from "./OnboardingArt.tsx";
import styles from "./Onboarding.module.scss";

const levels: Array<{
  level: BookLevel;
  label: string;
  description: string;
}> = [
  {
    level: "green",
    label: "Просто",
    description: "короткие или детские, подойдут в качестве первой книги",
  },
  {
    level: "yellow",
    label: "Средняя сложность",
    description:
      "подойдут для самостоятельного чтения, но могут быть длинными или сложными",
  },
  {
    level: "red",
    label: "Сложно",
    description: "серьезный challenge, может встретиться много незнакомых слов",
  },
];

type Step = {
  art: ReactNode;
  title: string;
  text: string;
};

const borrowSteps: Step[] = [
  {
    art: <ShelfArt direction="out" />,
    title: "Возьми книгу с полки",
    text: "В каталоге на сайте видно, какие книги свободны — можно выбрать заранее.",
  },
  {
    art: <PhoneScanArt />,
    title: "Отсканируй штрих-код",
    text: "Нажми кнопку «Сканировать книгу» и наведи камеру на штрих-код на задней обложке.",
  },
  {
    art: <DueDateArt />,
    title: "Готово!",
    text: "На экране появится дата, до которой книгу нужно вернуть.",
  },
];

const returnSteps: Step[] = [
  {
    art: <ShelfCodeArt />,
    title: "Отсканируй код на шкафчике",
    text: "Нажми кнопку «Вернуть» и наведи камеру на QR-код сбоку на шкафчике с книгами.",
  },
  {
    art: <BookBarcodeArt />,
    title: "Потом — штрих-код книги",
    text: "Тот же, что при получении: на задней обложке.",
  },
  {
    art: <ShelfArt direction="in" />,
    title: "Поставь книгу на полку",
    text: "И отметь, как прошло: дочитал(а) или было слишком сложно или скучно. Это увидит только учитель.",
  },
];

function Steps({ steps }: { steps: Step[] }) {
  return (
    <ol className={styles.steps}>
      {steps.map(({ art, title, text }, index) => (
        <li className={styles.step} key={title}>
          {art}
          <div>
            <h3 className={styles.stepTitle}>
              <span className={styles.stepNumber}>{index + 1}</span>
              {title}
            </h3>
            <p className={styles.stepText}>{text}</p>
          </div>
        </li>
      ))}
    </ol>
  );
}

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
          Маленькая инструкция, прежде чем взять книгу.
        </p>

        <section className={styles.section}>
          <Eyebrow as="h2">Как взять книгу</Eyebrow>
          <Steps steps={borrowSteps} />
          <p className={styles.note}>
            Камера не читает код? Введи ISBN-код — 13 цифр под штрих-кодом.
          </p>
        </section>

        <section className={styles.section}>
          <Eyebrow as="h2">Как вернуть книгу</Eyebrow>
          <Steps steps={returnSteps} />
          <p className={styles.note}>
            Что-то не сканируется? Нажми «Вернуть без сканирования». Книгу всё
            равно примут, учитель проверит полку сам.
          </p>
        </section>

        <section className={styles.section}>
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
          <p className={styles.note}>
            На сайте цвет корешка на обложке = уровень. Ориентируйся по нему при
            выборе книги.
          </p>
        </section>

        <div className={styles.rules}>
          <section className={styles.rule}>
            <strong className={styles.number}>1</strong>
            <div>
              <h2 className={styles.ruleTitle}>книга на руках</h2>
              <p className={styles.ruleText}>
                Перед тем как взять новую книгу, нужно вернуть предыдущую.
              </p>
            </div>
          </section>
          <section className={styles.rule}>
            <strong className={styles.number}>21</strong>
            <div>
              <h2 className={styles.ruleTitle}>день на чтение</h2>
              <p className={styles.ruleText}>
                Если не успеваешь, попроси учителя продлить срок. Но постарайся
                уложиться в 3 недели.
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
              : "Понятно, к книгам!"}
        </Button>
      </footer>
    </section>
  );
}
