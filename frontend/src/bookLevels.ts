import type { BookLevel } from "./api.ts";

export const bookLevels: Array<{
  value: BookLevel;
  number: 1 | 2 | 3;
  shortLabel: string;
  label: string;
  hint: string;
  detail: string;
}> = [
  {
    value: "green",
    number: 1,
    shortLabel: "Просто",
    label: "Просто",
    hint: "адаптированные",
    detail: "Первый уровень: адаптированный или короткий текст.",
  },
  {
    value: "yellow",
    number: 2,
    shortLabel: "Средняя",
    label: "Средняя сложность",
    hint: "современная проза",
    detail: "Второй уровень: современная проза средней сложности.",
  },
  {
    value: "red",
    number: 3,
    shortLabel: "Сложно",
    label: "Сложно",
    hint: "длинные, архаичные",
    detail:
      "Третий уровень: длинно и местами архаично. Если бросишь на середине — это нормально, отметь причину при возврате.",
  },
];

export const bookLevel = (level: BookLevel) => {
  return bookLevels.find((item) => item.value === level) ?? bookLevels[0];
};

export const levelLabel = (level: BookLevel) => bookLevel(level).label;
