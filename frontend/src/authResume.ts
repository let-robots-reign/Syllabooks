const authReturnPathKey = "syllabooks:auth-return-path";

export function rememberAuthReturnPath(path: string): void {
  if (path.startsWith("/") && !path.startsWith("//")) {
    try {
      sessionStorage.setItem(authReturnPathKey, path);
    } catch {
      // A blocked storage API must not prevent the sign-in screen from opening.
    }
  }
}

export function takeAuthReturnPath(): string | null {
  try {
    const path = sessionStorage.getItem(authReturnPathKey);
    sessionStorage.removeItem(authReturnPathKey);
    return path?.startsWith("/") && !path.startsWith("//") ? path : null;
  } catch {
    return null;
  }
}
