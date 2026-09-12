import { useEffect, useState } from "react";
import {
  api,
  ApiError,
  clearToken,
  consumeOAuthRedirect,
  loadToken,
  saveToken,
} from "./api.ts";
import styles from "./App.module.scss";
import { Login } from "./Login.tsx";

// Runs once, before the first render, so the token doesn't linger in the URL.
const oauthError = consumeOAuthRedirect();

type Me = { id: string; display_name: string; is_admin: boolean };

// A bare signed-in / signed-out switch, enough to test auth. Routing and the
// app shell come in session 3.
function App() {
  const [token, setToken] = useState(loadToken);
  const [me, setMe] = useState<Me | null>(null);
  const [error, setError] = useState(oauthError);

  useEffect(() => {
    if (!token) return;
    let current = true;
    api<Me>("/me").then(
      (me) => {
        if (current) setMe(me);
      },
      (err: ApiError) => {
        if (!current) return;
        if (err.status === 401) setToken(null);
        else setError(err.message);
      },
    );
    return () => {
      current = false;
    };
  }, [token]);

  function signIn(newToken: string) {
    saveToken(newToken);
    setError(null);
    setToken(newToken);
  }

  async function signOut() {
    // Forget the token locally even when the server can't be reached; the
    // orphaned session row then simply expires.
    await api("/auth/logout", { method: "POST" }).catch(() => {});
    clearToken();
    setMe(null);
    setToken(null);
  }

  return (
    <>
      <header className={styles.header}>Syllabooks</header>
      <main className={styles.main}>
        {!token ? (
          <Login onSignedIn={signIn} initialError={error} />
        ) : me ? (
          <>
            <p>Привет, {me.display_name}!</p>
            {me.is_admin && <p>У тебя права администратора.</p>}
            <button onClick={signOut}>Выйти</button>
          </>
        ) : (
          <p>{error ?? "Загрузка…"}</p>
        )}
      </main>
    </>
  );
}

export default App;
