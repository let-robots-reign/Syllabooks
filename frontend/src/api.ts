import axios from "axios";

// The signed-in user, from GET /api/me.
export type Me = { id: string; display_name: string; is_admin: boolean };

export type BookLevel = "green" | "yellow" | "red";

export type Book = {
  id: string;
  isbn: string | null;
  title: string;
  author: string;
  level: BookLevel;
  page_count: number;
  description: string | null;
  cover_url: string | null;
};

export type CatalogBook = Book & {
  current_loan: {
    borrower_name: string;
    due_at: string;
  } | null;
};

export type CatalogResponse = {
  finished_count: number;
  books: CatalogBook[];
};

export type BookInput = {
  isbn: string;
  title: string;
  author: string;
  level: BookLevel;
  page_count: number;
  description: string;
};

export type BookLookup = {
  isbn: string;
  title: string;
  author: string;
  page_count: number;
  description: string;
  cover_preview: string | null;
  source: "openlibrary" | "google" | "openlibrary+google";
};

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// The browser sends the HttpOnly session cookie with same-origin API calls.
// JavaScript deliberately has no access to the session token.
const client = axios.create({
  baseURL: "/api",
  withCredentials: true,
});

export const api = async <T>(
  path: string,
  options: { method?: string; body?: unknown } = {},
): Promise<T> => {
  try {
    const response = await client.request<T>({
      url: path,
      method: options.method ?? "GET",
      data: options.body,
    });
    return response.data;
  } catch (error) {
    if (!axios.isAxiosError<{ error?: string }>(error)) throw error;

    const status = error.response?.status ?? 0;
    if (status === 0) {
      throw new ApiError(
        0,
        "Нет связи с сервером. Проверь интернет и попробуй ещё раз.",
      );
    }
    throw new ApiError(
      status,
      error.response?.data?.error ?? "Что-то пошло не так. Попробуй ещё раз.",
    );
  }
};

const oauthErrors: Record<string, string> = {
  cancelled: "Вход отменён.",
  banned: "Аккаунт заблокирован. Обратись к учителю.",
  failed: "Не получилось войти. Попробуй ещё раз.",
};

// OAuth errors use a fragment so they never reach server logs. Successful
// callbacks carry no token in the URL; the server has already set the cookie.
export const oauthRedirectError = (): string | null => {
  if (location.pathname !== "/auth/callback") return null;
  const params = new URLSearchParams(location.hash.slice(1));
  const error = params.get("error");
  return error ? (oauthErrors[error] ?? oauthErrors.failed) : null;
};
