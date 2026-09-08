import type { Campaign, Creative } from './api';

export async function checkDemoReadiness(
  kind: 'rules' | 'auction',
  loadCampaign: (id: string) => Promise<Campaign>,
  loadCreatives: (id: string) => Promise<{ items: Creative[] }>,
  now = Date.now(),
) {
  const ids =
    kind === 'auction'
      ? ['demo-auction-1', 'demo-auction-2', 'demo-auction-3']
      : ['demo-campaign-v1'];
  const checks = await Promise.all(
    ids.map(async (id) => {
      let campaign: Campaign;
      try {
        campaign = await loadCampaign(id);
      } catch (error) {
        if (
          typeof error === 'object' &&
          error &&
          'status' in error &&
          error.status === 404
        )
          return `${id} 已删除`;
        throw error;
      }
      if (campaign.status !== 'ACTIVE' || !campaign.activeVersion)
        return `${campaign.name} 未发布或已暂停`;
      if (campaign.slotId !== 'game-home-banner')
        return `${campaign.name} 的广告位已修改`;
      if (
        Date.parse(campaign.startAt) > now ||
        Date.parse(campaign.endAt) <= now
      )
        return `${campaign.name} 不在投放期内`;
      const creatives = await loadCreatives(id);
      if (!creatives.items.some((creative) => creative.status === 'ACTIVE'))
        return `${campaign.name} 没有启用的素材`;
      return null;
    }),
  );
  const problems = checks.filter((value): value is string => value !== null);
  if (problems.length === ids.length)
    throw new Error(
      `用例暂时不能投放：${problems.join('；')}。可到广告计划或素材管理查看。`,
    );
  return problems.length ? `已载入可用计划。${problems.join('；')}` : '';
}
