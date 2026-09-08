import type { Campaign, Creative } from './api';
import { firstAvailableNumber } from './available-number.ts';

export function selectedCreativeCampaignID(
  campaigns: Pick<Campaign, 'id'>[],
  requestedID: string,
) {
  return campaigns.some((campaign) => campaign.id === requestedID)
    ? requestedID
    : '';
}

export function nextTestCreativeNumber(items: Pick<Creative, 'title'>[]) {
  return firstAvailableNumber(
    items.map((item) => item.title),
    /^测试素材 (\d+)$/,
  );
}

export function testCreativeTitle(sequence: number) {
  return `测试素材 ${String(sequence).padStart(3, '0')}`;
}
