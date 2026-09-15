import { useEffect, useState } from "react";
import clsx from "clsx";
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router-dom";
import { api, ApiError, oauthRedirectError, type Me } from "./api.ts";
import styles from "./App.module.scss";
import { AdminBookForm } from "./admin/AdminBookForm.tsx";
import { AdminBooks } from "./admin/AdminBooks.tsx";
import { Home } from "./Home.tsx";
import { Login } from "./Login.tsx";
import { NotFound } from "./NotFound.tsx";
import { Privacy } from "./Privacy.tsx";
import { Profile } from "./Profile.tsx";
import { Scan } from "./scan/Scan.tsx";

function App() {
  const location = useLocation();
  const navigate = useNavigate();
  const [authAttempt, setAuthAttempt] = useState(0);
  const [me, setMe] = useState<Me | null>();
  const [error, setError] = useState(() => oauthRedirectError());

  useEffect(() => {
    let current = true;
    api<Me>("/me").then(
      (me) => {
        if (current) setMe(me);
      },
      (err: ApiError) => {
        if (!current) return;
        if (err.status === 401) setMe(null);
        else setError(err.message);
      },
    );
    return () => {
      current = false;
    };
  }, [authAttempt]);

  useEffect(() => {
    if (location.pathname === "/auth/callback") {
      navigate("/", { replace: true });
    }
  }, [location.pathname, navigate]);

  const signIn = () => {
    setError(null);
    setMe(undefined);
    setAuthAttempt((attempt) => attempt + 1);
  };

  const signOut = async () => {
    await api("/auth/logout", { method: "POST" }).catch(() => {});
    setMe(null);
    navigate("/", { replace: true });
  };

  const isPrivacy = location.pathname === "/privacy";
  const sand = me === null && !isPrivacy;
  const admin = me?.is_admin && location.pathname.startsWith("/admin/books");
  const scan = me != null && location.pathname === "/scan";

  let routes;
  if (me === undefined) {
    routes = (
      <Route
        path="*"
        element={<p className={styles.status}>{error ?? "Загрузка…"}</p>}
      />
    );
  } else if (me === null) {
    routes = (
      <Route
        path="*"
        element={<Login onSignedIn={signIn} initialError={error} />}
      />
    );
  } else {
    routes = (
      <>
        <Route
          path="/"
          element={
            me.is_admin ? (
              <Navigate to="/admin/books" replace />
            ) : (
              <Home me={me} />
            )
          }
        />
        <Route
          path="/profile"
          element={<Profile me={me} onSignOut={signOut} />}
        />
        <Route path="/scan" element={<Scan />} />
        {me.is_admin && (
          <>
            <Route path="/admin/books" element={<AdminBooks />} />
            <Route path="/admin/books/new" element={<AdminBookForm />} />
            <Route path="/admin/books/:id" element={<AdminBookForm />} />
          </>
        )}
        <Route path="*" element={<NotFound />} />
      </>
    );
  }

  return (
    <div className={clsx(styles.ground, sand && styles.sand)}>
      <div className={clsx(styles.column, admin && styles.adminColumn)}>
        <main
          className={clsx(
            styles.main,
            admin && styles.adminMain,
            scan && styles.scanMain,
          )}
        >
          <Routes>
            <Route path="/privacy" element={<Privacy />} />
            {routes}
          </Routes>
        </main>
      </div>
    </div>
  );
}

export default App;
