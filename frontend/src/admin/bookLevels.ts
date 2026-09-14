import type { BookLevel } from "../api.ts";

export const bookLevels: Array<{
  value: BookLevel;
  shortLabel: string;
  label: string;
  hint: string;
}> = [
  {
    value: "green",
    shortLabel: "Просто",
    label: "Просто",
    hint: "адаптированные",
  },
  {
    value: "yellow",
    shortLabel: "Средняя",
    label: "Средняя сложность",
    hint: "современная проза",
  },
  {
    value: "red",
    shortLabel: "Сложно",
    label: "Сложно",
    hint: "длинные, архаичные",
  },
];

export const levelLabel = (level: BookLevel) => {
  return bookLevels.find((item) => item.value === level)?.label ?? level;
};
