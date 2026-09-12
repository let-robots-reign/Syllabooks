import type { Me } from "./api.ts";
import { BackBar } from "./Header.tsx";
import styles from "./Page.module.scss";
import { Button } from "./ui/Button.tsx";

// The profile (PRD §9.8: current book, history, personal stats) comes in
// session 13. For now it shows who is signed in, and the way out.
export function Profile({ me, onSignOut }: { me: Me; onSignOut: () => void }) {
  return (
    <>
      <BackBar note="Видно только тебе" />
      <h1 className={styles.title}>{me.display_name}</h1>
      {me.is_admin && <p className={styles.meta}>Учитель</p>}
      <div className={styles.actions}>
        <Button variant="secondary" compact onClick={onSignOut}>
          Выйти
        </Button>
      </div>
    </>
  );
}
