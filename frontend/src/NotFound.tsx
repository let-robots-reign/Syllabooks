import { BackBar } from "./Header.tsx";
import styles from "./Page.module.scss";

export function NotFound() {
  return (
    <>
      <BackBar />
      <h1 className={styles.title}>Такой страницы нет</h1>
      <p className={styles.lead}>
        Возможно, ссылка устарела. Все книги — в каталоге.
      </p>
    </>
  );
}
