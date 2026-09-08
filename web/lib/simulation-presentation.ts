import type {
  SimulationConfig,
  SimulationSnapshot,
} from './user-pool-simulator';
export function simulationProgress(
  config: Pick<SimulationConfig, 'seconds' | 'maxRounds'>,
  snapshot: Pick<SimulationSnapshot, 'elapsedMs' | 'started' | 'status'> | null,
) {
  const elapsed = ((snapshot?.elapsedMs ?? 0) / 1000).toFixed(1);
  const settled =
    snapshot?.status === 'completed' || snapshot?.status === 'stopped';
  return {
    primary:
      '已发 ' +
      (snapshot?.started ?? 0) +
      ' / ' +
      (config.maxRounds ?? 3000) +
      ' 轮',
    secondary:
      (settled ? '实际用时' : '已用时') +
      ' ' +
      elapsed +
      ' 秒 · 最长派发 ' +
      config.seconds +
      ' 秒',
  };
}
