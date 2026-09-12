import { BackBar } from "./Header.tsx";
import styles from "./Page.module.scss";

// The privacy policy (PRD §9.10). Public: the sign-in screen and both OAuth
// providers link to it. The text is the teacher's to write before launch.
export function Privacy() {
  return (
    <>
      <BackBar />
      <h1 className={styles.title}>Политика конфиденциальности</h1>
      <p className={styles.lead}>
        Мы храним только имя и список книг. Полный текст появится здесь до
        открытия клуба.
      </p>
    </>
  );
}
