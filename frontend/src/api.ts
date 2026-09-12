// The session token lives in localStorage, not a cookie (PRD §8). It lasts a
// year: a student who has to log in again every week stops using the app.
const TOKEN_KEY = "syllabooks.token";

export function loadToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function saveToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// api calls the backend with the session token. A 401 on a request that
// carried a token means the session is gone (expired, deleted, or the user
// was banned), so the token is dropped. Errors carry copy ready to show.
export async function api<T>(
  path: string,
  options: { method?: string; body?: unknown } = {},
): Promise<T> {
  const token = loadToken();
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  if (options.body !== undefined) headers["Content-Type"] = "application/json";

  let res: Response;
  try {
    res = await fetch(`/api${path}`, {
      method: options.method ?? "GET",
      headers,
      body:
        options.body === undefined ? undefined : JSON.stringify(options.body),
    });
  } catch {
    throw new ApiError(
      0,
      "Нет связи с сервером. Проверь интернет и попробуй ещё раз.",
    );
  }
  if (res.status === 401 && token) clearToken();
  if (!res.ok) {
    const data = await res.json().catch(() => null);
    throw new ApiError(
      res.status,
      data?.error ?? "Что-то пошло не так. Попробуй ещё раз.",
    );
  }
  return res.status === 204 ? (undefined as T) : res.json();
}

const oauthErrors: Record<string, string> = {
  cancelled: "Вход отменён.",
  banned: "Аккаунт заблокирован. Обратись к учителю.",
  failed: "Не получилось войти. Попробуй ещё раз.",
};

// consumeOAuthRedirect handles the page a Yandex or VK login lands on,
// /auth/callback#token=… or #error=…. It saves the token, removes it from the
// address bar and history, and returns the error to show, if any.
export function consumeOAuthRedirect(): string | null {
  if (location.pathname !== "/auth/callback") return null;
  const params = new URLSearchParams(location.hash.slice(1));
  history.replaceState(null, "", "/");
  const token = params.get("token");
  if (token) {
    saveToken(token);
    return null;
  }
  return oauthErrors[params.get("error") ?? ""] ?? oauthErrors.failed;
}
