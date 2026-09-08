import type { Campaign } from './api';
import { adSlotLabel } from './ad-slots.ts';
import {
  campaignDisplayStatus,
  campaignDisplayLabels,
} from './campaign-delivery.ts';

export const allCampaignFilters = '__all__';
export const campaignStatusOptions = [
  { value: allCampaignFilters, label: '全部状态' },
  { value: 'DRAFT', label: '草稿' },
  { value: 'ACTIVE', label: '投放中' },
  { value: 'SCHEDULED', label: '待开始' },
  { value: 'PAUSED', label: '已暂停' },
  { value: 'ENDED', label: '已结束' },
  { value: 'INVALID_PERIOD', label: '时间异常' },
];

export function filterCampaigns(
  campaigns: Campaign[],
  query: string,
  slotId = allCampaignFilters,
  status = allCampaignFilters,
  now: number | null = Date.now(),
): Campaign[] {
  const search = query.trim().toLocaleLowerCase();
  return campaigns.filter(
    (campaign) =>
      (slotId === allCampaignFilters || campaign.slotId === slotId) &&
      (status === allCampaignFilters ||
        campaignDisplayStatus(campaign, now) === status) &&
      (!search ||
        [
          campaign.name,
          campaign.slotId,
          adSlotLabel(campaign.slotId),
          campaign.status,
          campaignDisplayLabels[campaignDisplayStatus(campaign, now)],
        ].some((value) => value.toLocaleLowerCase().includes(search))),
  );
}
