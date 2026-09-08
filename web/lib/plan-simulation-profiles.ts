import type { Campaign, Condition, Profile } from './api';
import {
  fieldValueOptions,
  numericFieldLimits,
  validateFieldCondition,
  validNumericField,
} from './profile-options.ts';
import { makeSimulationProfiles } from './user-pool-simulator.ts';

export type SampleMix = 'matched' | 'unmatched' | 'mixed';
type Rule = NonNullable<Campaign['activeVersion']>['targeting'];
type Constraint = { condition: Condition; expected: boolean };
export interface PlanSampleContext {
  campaignId: string;
  campaignName: string;
  version: number;
  slotId: string;
  targeting: Rule;
  mix: SampleMix;
  expected: Record<string, boolean>;
  warnings: string[];
}

function conditionMatches(profile: Profile, condition: Condition): boolean {
  if (condition.tag) return profile.tags.includes(condition.tag);
  const field = condition.field!;
  if (!Object.hasOwn(profile.fields, field)) return false;
  const actual = profile.fields[field];
  const values =
    condition.op === 'in'
      ? condition.value!.split(',').map((v) => v.trim())
      : [condition.value!];
  if (numericFieldLimits(field)) {
    if (!validNumericField(field, actual)) return false;
    if (condition.op === 'gte') return Number(actual) >= Number(values[0]);
    if (condition.op === 'lte') return Number(actual) <= Number(values[0]);
    return values.some((value) => Number(actual) === Number(value));
  }
  return values.includes(actual);
}

function validateRule(rule: Rule) {
  const conditions = [
    ...(rule.all ?? []),
    ...(rule.any ?? []),
    ...(rule.none ?? []),
  ];
  if (conditions.length > 50) throw new Error('暂不支持超过 50 条条件的计划');
  for (const condition of conditions) {
    if (condition.tag) {
      if (Array.from(condition.tag).length > 64)
        throw new Error('计划标签超过长度限制');
    } else {
      if (
        !condition.field ||
        condition.field !== condition.field.trim() ||
        !condition.op ||
        !condition.value
      )
        throw new Error('计划含有不完整的定向条件，请先修正规则');
      validateFieldCondition(condition.field, condition.op, condition.value);
    }
  }
}

// Mirrors the backend's flat all / any / none predicates for sample labeling.
// These labels never replace the backend's decision or eligibility checks.
export function matchesSampleTargeting(profile: Profile, rule: Rule): boolean {
  validateRule(rule);
  return (
    (rule.all ?? []).every((c) => conditionMatches(profile, c)) &&
    (!rule.any?.length || rule.any.some((c) => conditionMatches(profile, c))) &&
    !(rule.none ?? []).some((c) => conditionMatches(profile, c))
  );
}

function branches(rule: Rule, matched: boolean): Constraint[][] {
  const yes = (condition: Condition) => ({ condition, expected: true });
  const no = (condition: Condition) => ({ condition, expected: false });
  if (matched) {
    const required = [
      ...(rule.all ?? []).map(yes),
      ...(rule.none ?? []).map(no),
    ];
    return rule.any?.length
      ? rule.any.map((c) => [...required, yes(c)])
      : [required];
  }
  return [
    ...(rule.all ?? []).map((c) => [no(c)]),
    ...(rule.any?.length ? [rule.any.map(no)] : []),
    ...(rule.none ?? []).map((c) => [yes(c)]),
  ];
}

function randomIndex(seed: string, key: string, length: number) {
  let hash = 2166136261;
  for (const char of seed + ':' + key)
    hash = Math.imul(hash ^ char.charCodeAt(0), 16777619) >>> 0;
  hash ^= hash << 13;
  hash ^= hash >>> 17;
  hash ^= hash << 5;
  return (hash >>> 0) % length;
}

function decimalText(value: number) {
  const text = String(value);
  if (!text.includes('e')) return text;
  const [mantissa, exponent] = text.split('e');
  const digits = mantissa.replace('.', '');
  const point =
    (mantissa.indexOf('.') < 0 ? mantissa.length : mantissa.indexOf('.')) +
    Number(exponent);
  return point <= 0
    ? '0.' + '0'.repeat(-point) + digits
    : point >= digits.length
      ? digits + '0'.repeat(point - digits.length)
      : digits.slice(0, point) + '.' + digits.slice(point);
}

function fieldDomain(
  field: string,
  conditions: Condition[],
  existing?: string,
): (string | undefined)[] {
  const values = conditions.flatMap((c) =>
    c.op === 'in' ? c.value!.split(',').map((v) => v.trim()) : [c.value!],
  );
  const limits = numericFieldLimits(field);
  let domain: string[];
  if (limits?.integer) {
    domain = Array.from({ length: limits.max - limits.min + 1 }, (_, i) =>
      String(i + limits.min),
    );
  } else if (limits) {
    // Predicates only change at their numeric boundaries. Include each boundary
    // and a representative of every open interval, including narrow decimals.
    const points = [
      ...new Set([
        limits.min,
        limits.max,
        ...values.map(Number),
        ...(existing ? [Number(existing)] : []),
      ]),
    ]
      .filter((v) => Number.isFinite(v) && v >= limits.min && v <= limits.max)
      .sort((a, b) => a - b);
    domain = points.flatMap((value, i) =>
      i
        ? [
            decimalText(value),
            decimalText(points[i - 1] + (value - points[i - 1]) / 2),
          ]
        : [decimalText(value)],
    );
    domain = domain.filter((value) => validNumericField(field, value));
  } else if (fieldValueOptions(field)) {
    domain = fieldValueOptions(field)!.map((option) => option.value);
  } else {
    let other = 'sample-other';
    while (values.includes(other)) other += '-x';
    domain = [...values, other];
  }
  return [
    ...new Set([...(existing !== undefined ? [existing] : []), ...domain]),
    undefined,
  ];
}

function solve(
  base: Profile,
  constraints: Constraint[],
  seed: string,
): Profile | undefined {
  const profile = structuredClone(base);
  const groups = new Map<string, Constraint[]>();
  for (const constraint of constraints) {
    const { condition } = constraint;
    const key = condition.tag
      ? 'tag:' + condition.tag
      : 'field:' + condition.field;
    groups.set(key, [...(groups.get(key) ?? []), constraint]);
  }
  for (const [key, group] of groups) {
    const tag = group[0].condition.tag;
    if (tag) {
      const required = group[0].expected;
      if (group.some((c) => c.expected !== required)) return;
      profile.tags = profile.tags.filter((value) => value !== tag);
      if (required) profile.tags.push(tag);
    } else {
      const field = group[0].condition.field!;
      const domain = fieldDomain(
        field,
        group.map((c) => c.condition),
        Object.hasOwn(profile.fields, field)
          ? profile.fields[field]
          : undefined,
      );
      const allowed = domain.filter((value) => {
        const fields = { ...profile.fields };
        if (value === undefined) delete fields[field];
        else
          Object.defineProperty(fields, field, {
            value,
            enumerable: true,
            configurable: true,
            writable: true,
          });
        return group.every(
          (c) =>
            conditionMatches({ ...profile, fields }, c.condition) ===
            c.expected,
        );
      });
      if (!allowed.length) return;
      // Prefer complete profiles; missing fields are used only when necessary.
      const present = allowed.filter((value) => value !== undefined);
      const choices = present.length ? present : allowed;
      const value =
        choices[randomIndex(seed, base.userId + ':' + key, choices.length)];
      if (value === undefined) delete profile.fields[field];
      else
        Object.defineProperty(profile.fields, field, {
          value,
          enumerable: true,
          configurable: true,
          writable: true,
        });
    }
  }
  return profile;
}

export function makePlanSimulationProfiles(
  count: number,
  seed: string,
  campaign: Campaign,
  mix: SampleMix,
) {
  const base = makeSimulationProfiles(count, seed);
  if (!['matched', 'unmatched', 'mixed'].includes(mix))
    throw new Error('请选择有效的样本组合');
  if (!campaign.activeVersion)
    throw new Error('该计划尚未发布规则，请先发布后再生成样本');
  const rule = campaign.activeVersion.targeting;
  validateRule(rule);
  const expected: Record<string, boolean> = {};
  const profiles = base.map((profile, i) => {
    const matched = mix === 'matched' || (mix === 'mixed' && i % 2 === 0);
    const alternatives = branches(rule, matched);
    const offset = alternatives.length
      ? randomIndex(seed, profile.userId + ':branch', alternatives.length)
      : 0;
    for (let b = 0; b < alternatives.length; b++) {
      const candidate = solve(
        profile,
        alternatives[(b + offset) % alternatives.length],
        seed,
      );
      if (candidate && matchesSampleTargeting(candidate, rule) === matched) {
        expected[profile.userId] = matched;
        return candidate;
      }
    }
    throw new Error(
      matched
        ? '无法生成满足定向的样本：计划条件可能互相矛盾，请检查“必须全部满足”和排除条件。'
        : '无法生成不满足定向的样本：该计划没有可违反的定向条件，请选择“全部满足”。',
    );
  });
  return {
    profiles,
    context: {
      campaignId: campaign.id,
      campaignName: campaign.name,
      version: campaign.activeVersion.number,
      slotId: campaign.slotId,
      targeting: structuredClone(rule),
      mix,
      expected,
      warnings: [],
    } satisfies PlanSampleContext,
  };
}
