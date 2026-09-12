import { useEffect, useState, type ReactNode } from "react";
import {
  api,
  ApiError,
  clearToken,
  consumeOAuthRedirect,
  loadToken,
  saveToken,
  type Me,
} from "./api.ts";
import styles from "./App.module.scss";
import { Home } from "./Home.tsx";
import { Login } from "./Login.tsx";
import { NotFound } from "./NotFound.tsx";
import { Privacy } from "./Privacy.tsx";
import { Profile } from "./Profile.tsx";
import { navigate, usePath } from "./router.ts";
import { cx } from "./ui/cx.ts";

// Runs once, before the first render, so the token doesn't linger in the URL.
const oauthError = consumeOAuthRedirect();

function App() {
  const path = usePath();
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
    navigate("/", { replace: true });
  }

  // The sign-in screens stand on sand, as in the design; the rest on cream.
  let screen: ReactNode;
  let sand = false;
  if (path === "/privacy") {
    screen = <Privacy />;
  } else if (!token) {
    screen = <Login onSignedIn={signIn} initialError={error} />;
    sand = true;
  } else if (!me) {
    screen = <p className={styles.status}>{error ?? "Загрузка…"}</p>;
  } else {
    screen = route(path, me, signOut);
  }

  return (
    <div className={cx(styles.ground, sand && styles.sand)}>
      <div className={styles.column}>
        <main className={styles.main}>{screen}</main>
      </div>
    </div>
  );
}

// route picks the screen for a signed-in user's path.
function route(path: string, me: Me, signOut: () => void): ReactNode {
  switch (path) {
    case "/":
      return <Home me={me} />;
    case "/profile":
      return <Profile me={me} onSignOut={signOut} />;
    default:
      return <NotFound />;
  }
}

export default App;
