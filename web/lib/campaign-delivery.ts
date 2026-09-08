import type { Campaign } from './api';

export type CampaignDisplayStatus =
  | Campaign['status']
  | 'SCHEDULED'
  | 'INVALID_PERIOD'
  | 'CHECKING';
export const campaignDisplayLabels: Record<CampaignDisplayStatus, string> = {
  DRAFT: '草稿',
  ACTIVE: '投放中',
  PAUSED: '已暂停',
  ENDED: '已结束',
  SCHEDULED: '待开始',
  INVALID_PERIOD: '时间异常',
  CHECKING: '检查中',
};

// Display state is derived; never replace the persisted lifecycle status.
export function campaignDisplayStatus(
  campaign: Pick<Campaign, 'status' | 'startAt' | 'endAt'>,
  now: number | null,
): CampaignDisplayStatus {
  if (campaign.status === 'DRAFT' || campaign.status === 'ENDED')
    return campaign.status;
  const start = Date.parse(campaign.startAt),
    end = Date.parse(campaign.endAt);
  if (!Number.isFinite(start) || !Number.isFinite(end) || start >= end)
    return 'INVALID_PERIOD';
  if (now === null || !Number.isFinite(now))
    return campaign.status === 'PAUSED' ? 'PAUSED' : 'CHECKING';
  if (now >= end) return 'ENDED';
  if (campaign.status === 'PAUSED') return 'PAUSED';
  return now < start ? 'SCHEDULED' : 'ACTIVE';
}

export function nextCampaignClockDelay(
  campaigns: Pick<Campaign, 'status' | 'startAt' | 'endAt'>[],
  now: number,
) {
  let delay = 60_000;
  for (const campaign of campaigns) {
    if (campaign.status !== 'ACTIVE' && campaign.status !== 'PAUSED') continue;
    for (const value of [campaign.startAt, campaign.endAt]) {
      const boundary = Date.parse(value);
      if (boundary > now) delay = Math.min(delay, boundary - now);
    }
  }
  return Math.max(1, delay);
}

const dateFormat = new Intl.DateTimeFormat('zh-CN', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
export function formatCampaignDate(value: string) {
  const time = Date.parse(value);
  return Number.isFinite(time)
    ? dateFormat.format(time).replaceAll('/', '-')
    : '时间无效';
}
