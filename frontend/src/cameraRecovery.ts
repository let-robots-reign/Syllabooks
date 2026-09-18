export type CameraFailure = "denied" | "unavailable";

export const cameraRecognitionTimeoutMs = 20_000;

type NavigatorDetails = Pick<
  Navigator,
  "userAgent" | "platform" | "maxTouchPoints"
>;

export function cameraRecoverySteps(
  details: NavigatorDetails = navigator,
): string[] {
  const userAgent = details.userAgent;
  const appleMobile =
    /iPhone|iPad|iPod/i.test(userAgent) ||
    (details.platform === "MacIntel" && details.maxTouchPoints > 1);

  if (appleMobile) {
    const safari =
      /Safari/i.test(userAgent) &&
      !/CriOS|FxiOS|EdgiOS|OPiOS|DuckDuckGo/i.test(userAgent);

    if (safari) {
      return [
        "В Safari нажми меню страницы слева от адреса и открой «Настройки веб-сайта».",
        "Для «Камера» выбери «Разрешить».",
        "Если настройки сайта нет: «Настройки» → «Приложения» → Safari → «Камера» → «Разрешить».",
      ];
    }

    return [
      "Открой «Настройки» → «Конфиденциальность и безопасность» → «Камера».",
      "Разреши доступ браузеру, в котором открыт Syllabooks.",
      "Вернись в браузер, разреши камеру для сайта и обнови страницу.",
    ];
  }

  if (/Android/i.test(userAgent)) {
    return [
      "Нажми значок настроек рядом с адресом сайта.",
      "Открой «Разрешения» → «Камера» → «Разрешить».",
      "Обнови страницу.",
    ];
  }

  const desktopSafari =
    /Safari/i.test(userAgent) && !/Chrome|Chromium|Edg/i.test(userAgent);
  if (desktopSafari) {
    return [
      "Открой Safari → «Настройки» → «Веб-сайты» → «Камера».",
      "Напротив Syllabooks выбери «Разрешить».",
      "Обнови страницу.",
    ];
  }

  return [
    "Открой настройки сайта рядом с адресом.",
    "Выбери «Камера» → «Разрешить».",
    "Обнови страницу.",
  ];
}
