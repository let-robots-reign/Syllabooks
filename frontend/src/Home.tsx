import type { Me } from "./api.ts";
import { Header } from "./Header.tsx";
import styles from "./Home.module.scss";

// The main page. The catalogue (PRD §9.3: class counter, level filter, book
// cards) fills it in session 7; until then it shows the design's first-run
// state of the shelf.
export function Home({ me }: { me: Me }) {
  return (
    <>
      <Header name={me.display_name} />
      <section className={styles.shelf}>
        <h1 className={styles.heading}>На полке</h1>
        <div className={styles.hatch} />
        <h2 className={styles.title}>Полка пока пустая</h2>
        <p className={styles.text}>
          Учитель добавляет книги по штрих-кодам. Зайди попозже.
        </p>
      </section>
    </>
  );
}
