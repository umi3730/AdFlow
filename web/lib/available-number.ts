// Display defaults only, never a substitute for backend primary keys.
export function firstAvailableNumber(values: string[], pattern: RegExp) {
  const used = new Set<number>();
  for (const value of values) {
    const match = pattern.exec(value);
    if (!match) continue;
    const number = Number(match[1]);
    if (Number.isSafeInteger(number) && number > 0) used.add(number);
  }
  let next = 1;
  while (used.has(next)) next++;
  return next;
}

export async function collectNumberingPages<T>(
  fetchPage: (offset: number, limit: number) => Promise<{ items: T[] }>,
): Promise<T[]> {
  const items: T[] = [];
  const limit = 100;
  for (let offset = 0; offset < 100000; offset += limit) {
    const page = await fetchPage(offset, limit);
    items.push(...page.items);
    if (page.items.length < limit) return items;
  }
  throw new Error('数据量超出默认编号扫描范围，请手动指定编号');
}
