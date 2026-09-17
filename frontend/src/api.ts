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
  my_loan: LoanDetail | null;
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

export type BorrowResponse = {
  id: string;
  taken_at: string;
  due_at: string;
  book: Book;
};

export type ReturnReason = "finished" | "too_hard" | "boring" | "skipped";

export type LoanDetail = {
  id: string;
  taken_at: string;
  due_at: string;
  returned_at: string | null;
  return_reason: ReturnReason | null;
  shelf_scan_ok: boolean | null;
  book_scan_ok: boolean | null;
  book: Book;
};

export type FinishCelebration = {
  class_finished_count: number;
  is_first_book: boolean;
};

export type ReturnReasonResponse = {
  loan: LoanDetail;
  celebration: FinishCelebration | null;
};

export type AdminStats = {
  open_loans: number;
  books: number;
  lost_books: number;
  users: number;
};

export type AdminUserStatus = "approved" | "pending" | "banned";
export type AdminUserLoginMethod = "code" | "yandex" | "vk";

export type AdminUser = {
  id: string;
  display_name: string;
  login_method: AdminUserLoginMethod;
  code: string | null;
  has_password: boolean;
  status: AdminUserStatus;
  created_at: string;
  current_loan: {
    id: string;
    due_at: string;
    book: Pick<Book, "id" | "title" | "level">;
  } | null;
  finished_count: number;
  abandoned_count: number;
};

export type AdminUsersResponse = {
  as_of: string;
  summary: {
    finished_books: number;
    reading_now: number;
    without_book: number;
    never_borrowed: number;
  };
  users: AdminUser[];
};

export type IssuedCodeResponse = {
  id: string;
  display_name?: string;
  code: string;
};

export type AdminLoanBook = Pick<
  Book,
  "id" | "isbn" | "title" | "level" | "page_count"
>;

export type AdminLoanStudent = {
  id: string;
  display_name: string;
};

export type AdminOpenLoan = {
  id: string;
  taken_at: string;
  due_at: string;
  book: AdminLoanBook;
  student: AdminLoanStudent;
};

export type AdminScanReview = AdminOpenLoan & {
  returned_at: string;
  shelf_scan_ok: boolean;
  book_scan_ok: boolean;
};

export type AdminLoansResponse = {
  as_of: string;
  open_loans: AdminOpenLoan[];
  review_queue: AdminScanReview[];
};

export type AdminLostBook = {
  book: AdminLoanBook;
  lost_at: string;
  last_borrower_name: string;
  last_taken_at: string | null;
};

export type AdminLostBooksResponse = {
  books: AdminLostBook[];
};

export type ReturnMethod = "scan" | "manual" | "skipped";

export type ReturnEvidence = {
  method: ReturnMethod;
  value: string;
};

export type BorrowErrorCode =
  | "invalid_isbn"
  | "book_not_found"
  | "book_lost"
  | "book_unavailable"
  | "loan_limit"
  | "no_open_loan"
  | "invalid_shelf_code"
  | "wrong_book"
  | "already_returned"
  | "invalid_return_evidence"
  | "invalid_return_reason"
  | "loan_not_returned"
  | "return_reason_set";

type ErrorPayload = {
  error?: string;
  code?: BorrowErrorCode;
  book?: Book;
  borrower_name?: string;
  due_at?: string;
};

export class ApiError extends Error {
  status: number;
  code?: BorrowErrorCode;
  book?: Book;
  borrowerName?: string;
  dueAt?: string;

  constructor(status: number, message: string, details: ErrorPayload = {}) {
    super(message);
    this.status = status;
    this.code = details.code;
    this.book = details.book;
    this.borrowerName = details.borrower_name;
    this.dueAt = details.due_at;
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
    if (!axios.isAxiosError<ErrorPayload>(error)) throw error;

    const status = error.response?.status ?? 0;
    if (status === 0) {
      throw new ApiError(
        0,
        "Нет связи с сервером. Проверь интернет и попробуй ещё раз.",
      );
    }
    const details = error.response?.data ?? {};
    throw new ApiError(
      status,
      details.error ?? "Что-то пошло не так. Попробуй ещё раз.",
      details,
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
