import type { Campaign, Condition } from './api';
import { validateFieldCondition } from './profile-options.ts';

export const ruleGroups = ['all', 'any', 'none'] as const;
export type RuleGroup = (typeof ruleGroups)[number];
export type Targeting = Record<RuleGroup, Condition[]>;
export interface RuleEditorValue {
  auction?: { advertiserId: string; advertiserName: string; bidYuan: string };
  targeting: Targeting;
  dailyBudgetYuan: string;
  impressionCostYuan: string;
  frequencyLimit: string;
}

export function editorFromCampaign(campaign: Campaign): RuleEditorValue {
  const version = campaign.activeVersion;
  return {
    ...(version?.auction
      ? {
          auction: {
            advertiserId: version.auction.advertiserId,
            advertiserName: version.auction.advertiserName,
            bidYuan: (version.auction.bidFen / 100).toFixed(2),
          },
        }
      : {}),
    targeting: {
      all: (
        version?.targeting.all ?? (version ? [] : [{ tag: 'tech_interest' }])
      ).map((item) => ({ ...item })),
      any: (version?.targeting.any ?? []).map((item) => ({ ...item })),
      none: (version?.targeting.none ?? []).map((item) => ({ ...item })),
    },
    dailyBudgetYuan: ((version?.dailyBudgetFen ?? 100000) / 100).toFixed(2),
    impressionCostYuan: ((version?.impressionCostFen ?? 100) / 100).toFixed(2),
    frequencyLimit: String(version?.frequencyLimit ?? 3),
  };
}

function yuanToFen(value: string, label: string): number {
  const text = value.trim();
  if (!/^\d+(\.\d{1,2})?$/.test(text))
    throw new Error(`${label}请输入正数，最多两位小数`);
  const [yuan, fraction = ''] = text.split('.');
  const fen = Number(yuan) * 100 + Number(fraction.padEnd(2, '0'));
  if (!Number.isSafeInteger(fen) || fen <= 0)
    throw new Error(`${label}必须大于 0 且不能超出安全范围`);
  return fen;
}

export function compileRuleDraft(value: RuleEditorValue) {
  const count = ruleGroups.reduce(
    (sum, group) => sum + value.targeting[group].length,
    0,
  );
  if (count < 1 || count > 50) throw new Error('请设置 1～50 条定向条件');
  const targeting = {} as Targeting;
  for (const group of ruleGroups) {
    targeting[group] = value.targeting[group].map((condition) => {
      if (condition.tag !== undefined) {
        const tag = condition.tag.trim();
        if (!tag) throw new Error('标签不能为空；不需要的条件请删除');
        return { tag };
      }
      const field = condition.field?.trim();
      const actual = condition.value?.trim();
      const op = condition.op;
      if (
        !field ||
        !actual ||
        !op ||
        !['eq', 'in', 'gte', 'lte'].includes(op)
      ) {
        throw new Error('字段条件需要完整的字段名、比较方式和值');
      }
      validateFieldCondition(field, op, actual);
      return { field, op, value: actual };
    });
  }
  const dailyBudgetFen = yuanToFen(value.dailyBudgetYuan, '日预算');
  let auction;
  if (value.auction) {
    const advertiserId = value.auction.advertiserId.trim().toLowerCase();
    const advertiserName = value.auction.advertiserName.trim();
    const bidFen = yuanToFen(value.auction.bidYuan, '单次曝光出价');
    if (!/^[a-z0-9][a-z0-9_-]{0,63}$/.test(advertiserId))
      throw new Error('广告主标识需为 1～64 位小写字母、数字、下划线或短横线');
    if (
      Array.from(advertiserName).length < 2 ||
      Array.from(advertiserName).length > 128
    )
      throw new Error('广告主名称需为 2～128 个字符');
    if (bidFen > 1000000 || bidFen > dailyBudgetFen)
      throw new Error('单次出价不能超过日预算或 ¥10000');
    auction = { advertiserId, advertiserName, bidFen };
  }
  const impressionCostFen =
    auction?.bidFen ?? yuanToFen(value.impressionCostYuan, '单次曝光成本');
  if (dailyBudgetFen < impressionCostFen)
    throw new Error('日预算不能低于单次曝光成本');
  if (!/^\d+$/.test(value.frequencyLimit))
    throw new Error('每日频控应为 1～100 的整数');
  const frequencyLimit = Number(value.frequencyLimit);
  if (frequencyLimit < 1 || frequencyLimit > 100)
    throw new Error('每日频控应为 1～100 的整数');
  return {
    targeting,
    dailyBudgetFen,
    impressionCostFen,
    frequencyLimit,
    ...(auction ? { auction } : {}),
  };
}

export function prepareAgentPlan(value: RuleEditorValue): RuleEditorValue {
  const validated = compileRuleDraft(value);
  return {
    targeting: validated.targeting,
    ...(validated.auction
      ? {
          auction: {
            advertiserId: validated.auction.advertiserId,
            advertiserName: validated.auction.advertiserName,
            bidYuan: (validated.auction.bidFen / 100).toFixed(2),
          },
        }
      : {}),
    dailyBudgetYuan: (validated.dailyBudgetFen / 100).toFixed(2),
    impressionCostYuan: (validated.impressionCostFen / 100).toFixed(2),
    frequencyLimit: String(validated.frequencyLimit),
  };
}
