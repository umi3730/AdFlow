import type { Condition, RuleDraft } from './api';
import type { RuleEditorValue } from './campaign-rules';
import {
  profileTagDictionary,
  profileFields,
  fieldValueOptions,
} from './profile-options.ts';

const tags: Record<string, string> = {
  shopping_interest: '购物兴趣',
  anime: '二次元兴趣',
  strategy_game: '策略游戏兴趣',
  installed_target_game: '已安装目标游戏',
  ...Object.fromEntries(profileTagDictionary.map((tag) => [tag.id, tag.label])),
};
const fields: Record<string, string> = {
  ...Object.fromEntries(profileFields.map((field) => [field.id, field.label])),
  platform: '平台（独立字段）',
};
const operators: Record<string, string> = {
  eq: '等于',
  in: '属于',
  gte: '大于等于',
  lte: '小于等于',
};

// Presentation only: never rename unknown IDs or silently change rule semantics.
export function describeCondition(condition: Condition): string {
  if (condition.tag !== undefined)
    return tags[condition.tag] ?? `标签「${condition.tag}」`;
  const field = condition.field ?? '';
  const value = condition.value ?? '';
  const dictionary = fieldValueOptions(field);
  const displayValue = dictionary
    ? value
        .split(',')
        .map(
          (item) =>
            dictionary.find((option) => option.value === item.trim())?.label ??
            item.trim(),
        )
        .join('、')
    : value;
  return `${fields[field] ?? `字段「${field}」`} ${operators[condition.op ?? ''] ?? condition.op ?? ''} ${displayValue}`;
}

export function editorFromAgentDraft(draft: RuleDraft): RuleEditorValue {
  return {
    targeting: {
      all: (draft.targeting.all ?? []).map((row) => ({ ...row })),
      any: (draft.targeting.any ?? []).map((row) => ({ ...row })),
      none: (draft.targeting.none ?? []).map((row) => ({ ...row })),
    },
    dailyBudgetYuan: (draft.dailyBudgetFen / 100).toFixed(2),
    impressionCostYuan: (draft.impressionCostFen / 100).toFixed(2),
    frequencyLimit: String(draft.frequencyLimit),
  };
}

export function draftSource(draft: RuleDraft): string {
  if (draft.fallback) return '已降级：本地规则解析';
  if (draft.provider === 'local-mock') return '本地规则解析（非大模型）';
  return draft.model ? `模型：${draft.model}` : `生成服务：${draft.provider}`;
}

export function warningLabel(warning: string): string {
  if (
    warning === 'Budget and cost are conservative defaults; adjust as needed.'
  )
    return '预算和单次成本由模型补全，请按实际投放需求调整。';
  return warning;
}
