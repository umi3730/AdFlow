import type { Profile } from './api';

export const demoCampaignID = 'demo-campaign-v1';
export const demoSlotID = 'game-home-banner';
export const demoUserCases = [
  { userId: 'demo-user-match', label: '演示 · 命中' },
  { userId: 'demo-user-excluded', label: '演示 · 排除' },
  { userId: 'demo-user-miss', label: '演示 · 未命中' },
] as const;

// Read existing records only. Deletions and user edits are never repaired here.
export async function loadDemoProfiles(load: (id: string) => Promise<Profile>) {
  const results = await Promise.all(
    demoUserCases.map(async ({ userId }) => {
      try {
        return await load(userId);
      } catch (error) {
        if (
          typeof error === 'object' &&
          error !== null &&
          'status' in error &&
          error.status === 404
        )
          return null;
        throw error;
      }
    }),
  );
  return results.filter((profile): profile is Profile => profile !== null);
}
