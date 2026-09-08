// Shared UI presets; keep the original ID compatible with existing campaigns.
export const adSlotOptions: { value: string; label: string }[] = [
  { value: 'game-home-banner', label: '游戏首页横幅' },
  { value: 'feed-recommendation', label: '信息流推荐' },
  { value: 'content-detail-bottom', label: '内容详情底部' },
  { value: 'sidebar-recommendation', label: '侧栏推荐' },
  { value: 'activity-popup', label: '活动弹窗' },
];

export const defaultAdSlotID = 'game-home-banner';

export function adSlotLabel(slotId: string): string {
  return adSlotOptions.find((slot) => slot.value === slotId)?.label ?? slotId;
}
