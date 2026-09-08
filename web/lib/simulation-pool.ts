import type { Profile } from './api';
import type { PlanSampleContext } from './plan-simulation-profiles';

export type PoolSource = 'temporary' | 'saved';
export interface SimulationPool {
  profiles: Profile[];
  selectedIds: string[];
  seed?: string;
  planSample?: PlanSampleContext;
}
export function refreshSimulationPool(
  pool: SimulationPool,
  profiles: Profile[],
): SimulationPool {
  const validIds = new Set(profiles.map((profile) => profile.userId));
  return {
    ...pool,
    profiles,
    selectedIds: pool.selectedIds.filter((id) => validIds.has(id)),
  };
}
