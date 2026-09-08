import type { Campaign } from './api';
import { firstAvailableNumber } from './available-number.ts';

export function newAgentCampaignInput(
  name: string,
  slotId: string,
  now = new Date(),
) {
  name = name.trim();
  slotId = slotId.trim();
  // Count Unicode code points to match Go's request validator (not UTF-16 units).
  const nameLength = Array.from(name).length;
  const slotLength = Array.from(slotId).length;
  if (nameLength < 2 || nameLength > 128)
    throw new Error('计划名称需要 2～128 个字符');
  if (slotLength < 2 || slotLength > 64)
    throw new Error('广告位需要 2～64 个字符');
  return {
    name,
    slotId,
    startAt: now.toISOString(),
    endAt: new Date(now.getTime() + 7 * 86400000).toISOString(),
  };
}

// Convenience defaults only; this sequence is not a business identifier.
export function nextTestPlanNumber(campaigns: Pick<Campaign, 'name'>[]) {
  return firstAvailableNumber(
    campaigns.map((campaign) => campaign.name),
    /^测试计划 (\d+)$/,
  );
}

export function testPlanName(sequence: number) {
  return `测试计划 ${String(sequence).padStart(3, '0')}`;
}
