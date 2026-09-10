export const profileTags = [
  { id: 'tech_interest', label: '数码兴趣' },
  { id: 'gaming_interest', label: '游戏兴趣' },
  { id: 'active_7d', label: '近 7 天活跃' },
  { id: 'new_user', label: '新用户' },
  { id: 'paying_user', label: '付费用户' },
];
export const profileTagDictionary = [
  ...profileTags,
  { id: 'adflow_demo', label: '基础演示人群' },
  { id: 'auction_demo', label: '竞价人群' },
  { id: 'demo_excluded', label: '排除演示人群' },
  { id: 'shopping_interest', label: '购物兴趣' },
  { id: 'anime', label: '二次元兴趣' },
  { id: 'strategy_game', label: '策略游戏兴趣' },
  { id: 'installed_target_game', label: '已安装目标游戏' },
];
export function profileTagLabel(id: string) {
  return profileTagDictionary.find((tag) => tag.id === id)?.label ?? id;
}

// Keep common choices concise while translating known existing tags.
// Truly unknown values retain FormSelect's original-value fallback.
export function ruleTagOptions(current: string) {
  const options = profileTags.map((tag) => ({
    value: tag.id,
    label: tag.label,
  }));
  const existing = profileTagDictionary.find((tag) => tag.id === current);
  if (existing && !options.some((option) => option.value === current)) {
    options.push({ value: existing.id, label: existing.label });
  }
  return options;
}
const legacyTagLabels = new Map([
  ['演示用户', 'adflow_demo'],
  ['竞价演示用户', 'auction_demo'],
  ['演示排除人群', 'demo_excluded'],
]);
export function profileTagID(input: string) {
  const value = input.trim();
  return (
    profileTagDictionary.find((tag) => tag.label === value)?.id ??
    legacyTagLabels.get(value) ??
    value
  );
}

export type ProfileTagKind =
  | 'interest'
  | 'behavior'
  | 'demo'
  | 'exclusion'
  | 'custom';

export function profileTagInfo(id: string): {
  label: string;
  kind: ProfileTagKind;
  description: string;
} {
  const label = profileTagLabel(id);
  if (id === 'demo_excluded') {
    return {
      label,
      kind: 'exclusion',
      description: '用于演示排除条件，仅在计划设置对应排除规则时生效。',
    };
  }
  if (id === 'adflow_demo' || id === 'auction_demo') {
    return {
      label,
      kind: 'demo',
      description: '内置演示使用的人群标签，不表示已经命中或成交。',
    };
  }
  if (
    [
      'tech_interest',
      'gaming_interest',
      'shopping_interest',
      'anime',
      'strategy_game',
    ].includes(id)
  ) {
    return {
      label,
      kind: 'interest',
      description: '兴趣标签；与计划中的定向条件分别匹配。',
    };
  }
  if (profileTagDictionary.some((tag) => tag.id === id)) {
    return { label, kind: 'behavior', description: '用户行为或状态标签。' };
  }
  return {
    label,
    kind: 'custom',
    description: '自定义或未收录标签，保留原始标识。',
  };
}
export function addProfileTags(current: string, input: string) {
  const existing = current
    .split(/[,，]/)
    .map((tag) => tag.trim())
    .filter(Boolean);
  const added = input
    .split(/[,，]/)
    .map(profileTagID)
    .filter(Boolean);
  return [...new Set([...existing, ...added])].join(',');
}
export function profileDeviceLabel(value: string) {
  return deviceOptions.find((option) => option.value === value)?.label ?? value;
}
export const profileFields = [
  { id: 'device', label: '设备', sample: 'android' },
  { id: 'score', label: '活跃分数', sample: '88' },
  { id: 'age', label: '年龄', sample: '25' },
  { id: 'member_level', label: '会员等级', sample: 'basic' },
  { id: 'channel', label: '来源渠道', sample: 'organic' },
];
export const memberOptions = [
  { value: 'basic', label: '普通会员' },
  { value: 'silver', label: '白银会员' },
  { value: 'gold', label: '黄金会员' },
  { value: 'diamond', label: '钻石会员' },
];
export const channelOptions = [
  { value: 'organic', label: '自然访问' },
  { value: 'paid', label: '广告投放' },
  { value: 'referral', label: '邀请推荐' },
];
export function fieldValueOptions(field: string) {
  return field === 'device'
    ? deviceOptions
    : field === 'member_level'
      ? memberOptions
      : field === 'channel'
        ? channelOptions
        : undefined;
}
export function numericFieldLimits(field: string) {
  if (field === 'score')
    return { min: 0, max: 100, step: 'any', integer: false };
  if (field === 'age') return { min: 0, max: 120, step: '1', integer: true };
  return undefined;
}
export const deviceOptions = [
  { value: 'android', label: '安卓' },
  { value: 'ios', label: 'iOS' },
  { value: 'windows', label: 'Windows' },
  { value: 'macos', label: 'macOS' },
  { value: 'web', label: '网页' },
];
export function fieldOperators(field: string) {
  return numericFieldLimits(field)
    ? (['eq', 'in', 'gte', 'lte'] as const)
    : (['eq', 'in'] as const);
}
export function validScore(value: string) {
  return (
    /^\d+(\.\d+)?$/.test(value.trim()) &&
    Number.isFinite(Number(value)) &&
    Number(value) >= 0 &&
    Number(value) <= 100
  );
}
export function validNumericField(field: string, value: string) {
  const limits = numericFieldLimits(field);
  if (!limits) return false;
  const pattern = limits.integer ? /^\d+$/ : /^\d+(\.\d+)?$/;
  const number = Number(value);
  return (
    pattern.test(value.trim()) &&
    Number.isFinite(number) &&
    number >= limits.min &&
    number <= limits.max
  );
}
export function validateFieldCondition(
  field: string,
  op: string,
  value: string,
) {
  if (!fieldOperators(field).some((allowed) => allowed === op))
    throw new Error('只有年龄和活跃分数支持大小比较；字典字段只能等于或属于');
  const values =
    op === 'in' ? value.split(',').map((item) => item.trim()) : [value.trim()];
  if (values.some((item) => !item)) throw new Error('比较值不能为空');
  const options = fieldValueOptions(field);
  if (
    options &&
    values.some((item) => !options.some((option) => option.value === item))
  )
    throw new Error('请选择字典中的有效值');
  if (
    numericFieldLimits(field) &&
    values.some((item) => !validNumericField(field, item))
  )
    throw new Error(
      field === 'age'
        ? '年龄必须为 0～120 的整数'
        : '活跃分数必须为 0～100 的数字',
    );
}
export function toggleProfileTag(current: string, tag: string) {
  const tags = new Set(
    current
      .split(/[,，]/)
      .map((value) => value.trim())
      .filter(Boolean),
  );
  if (tags.has(tag)) tags.delete(tag);
  else tags.add(tag);
  return [...tags].join(',');
}
