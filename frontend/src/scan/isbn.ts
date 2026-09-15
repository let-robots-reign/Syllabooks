const ISBN_LENGTH = 13;

// Grouping is only a reading aid. The stored and reported value is always the
// unformatted 13 digits, because ISBN registration-group lengths vary.
export const formatIsbn = (value: string): string => {
  const digits = isbnDigits(value);
  const groups = [3, 3, 3, 3, 1];
  const parts: string[] = [];
  let offset = 0;

  for (const size of groups) {
    const part = digits.slice(offset, offset + size);
    if (!part) break;
    parts.push(part);
    offset += size;
  }

  return parts.join("-");
};

export const isbnDigits = (value: string): string => {
  return value.replace(/\D/g, "").slice(0, ISBN_LENGTH);
};

export const isValidEan13 = (value: string): boolean => {
  if (!/^\d{13}$/.test(value)) return false;

  const expected = Number(value[12]);
  const sum = value
    .slice(0, 12)
    .split("")
    .reduce((total, digit, index) => {
      return total + Number(digit) * (index % 2 === 0 ? 1 : 3);
    }, 0);
  const checkDigit = (10 - (sum % 10)) % 10;

  return checkDigit === expected;
};

export const manualIsbnError = (value: string): string | null => {
  if (value.length < ISBN_LENGTH) return null;
  if (!value.startsWith("978") && !value.startsWith("979")) {
    return "ISBN книги начинается с 978 или 979. Проверь цифры под штрих-кодом.";
  }
  if (!isValidEan13(value)) {
    return "Последняя цифра не сходится. Проверь ISBN на задней обложке.";
  }
  return null;
};
