const plural = (value: number, one: string, few: string, many: string) => {
  const mod100 = Math.abs(value) % 100;
  const mod10 = mod100 % 10;
  if (mod100 >= 11 && mod100 <= 14) return many;
  if (mod10 === 1) return one;
  if (mod10 >= 2 && mod10 <= 4) return few;
  return many;
};

export const bookWord = (value: number) =>
  plural(value, "книга", "книги", "книг");

export const booksReadWord = (value: number) =>
  plural(value, "книгу", "книги", "книг");

export const pageWord = (value: number) =>
  plural(value, "страница", "страницы", "страниц");

const dueDate = new Intl.DateTimeFormat("ru-RU", {
  day: "numeric",
  month: "long",
});

export const formatDueDate = (value: string) => dueDate.format(new Date(value));
