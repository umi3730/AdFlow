export interface SimulationBehavior {
  clickPercent: number;
  conversionPercent: number;
  minValueFen: number;
  maxValueFen: number;
}

export function validateSimulationBehavior(
  input: SimulationBehavior,
): SimulationBehavior {
  for (const percent of [input.clickPercent, input.conversionPercent]) {
    if (!Number.isInteger(percent) || percent < 0 || percent > 100)
      throw new Error('点击和转化概率需要在 0～100% 之间，使用整数');
  }
  for (const value of [input.minValueFen, input.maxValueFen]) {
    if (!Number.isSafeInteger(value) || value < 1 || value > 1000000)
      throw new Error('单次转化金额需要在 0.01～10000 元之间');
  }
  if (input.minValueFen > input.maxValueFen)
    throw new Error('转化金额下限不能大于上限');
  return { ...input };
}

export function simulationMoneyFen(input: string): number {
  const value = input.trim();
  if (!/^\d+(\.\d{1,2})?$/.test(value))
    throw new Error('金额请输入数字，最多两位小数');
  const [yuan, cents = ''] = value.split('.');
  const fen = Number(yuan) * 100 + Number(cents.padEnd(2, '0'));
  if (!Number.isSafeInteger(fen) || fen < 1 || fen > 1000000)
    throw new Error('单次转化金额需要在 0.01～10000 元之间');
  return fen;
}

// Seeded by the unique client request ID so exported runs are reproducible,
// independent of response ordering. This is synthetic data, not real revenue.
export function simulationBehaviorForRequest(
  config: SimulationBehavior,
  requestId: string,
) {
  let seed = 2166136261;
  for (const char of requestId)
    seed = Math.imul(seed ^ char.charCodeAt(0), 16777619) >>> 0;
  if (!seed) seed = 1;
  const random = () => {
    seed ^= seed << 13;
    seed ^= seed >>> 17;
    seed ^= seed << 5;
    return (seed >>> 0) / 4294967296;
  };
  const click = random() * 100 < config.clickPercent;
  const convert = click && random() * 100 < config.conversionPercent;
  return {
    click,
    valueFen: convert
      ? config.minValueFen +
        Math.floor(random() * (config.maxValueFen - config.minValueFen + 1))
      : null,
  };
}
