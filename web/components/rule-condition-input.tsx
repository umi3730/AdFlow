'use client';
import type { Condition } from '@/lib/api';
import {
  fieldValueOptions,
  numericFieldLimits,
  fieldOperators,
  profileFields,
  profileTags,
} from '@/lib/profile-options';
import { Input } from '@/components/ui/input';
import { FormSelect } from '@/components/form-select';
import { DictionaryValueInput } from '@/components/dictionary-value-input';

const opLabels = { eq: '等于', in: '属于', gte: '大于等于', lte: '小于等于' };
export function RuleConditionInput({
  row,
  onChange,
  label,
  disabled,
}: {
  row: Condition;
  onChange: (row: Condition) => void;
  label: string;
  disabled: boolean;
}) {
  const field = row.field ?? 'device';
  const allowed = fieldOperators(field);
  const validOp = allowed.some((op) => op === row.op);
  const dictionary = fieldValueOptions(field);
  const limits = numericFieldLimits(field);
  return (
    <>
      <div className="w-28 shrink-0">
        <FormSelect
          label={label + '类型'}
          value={row.tag !== undefined ? 'tag' : 'field'}
          options={[
            { value: 'tag', label: '用户标签' },
            { value: 'field', label: '画像字段' },
          ]}
          disabled={disabled}
          onChange={(type) =>
            onChange(
              type === 'tag'
                ? { tag: 'tech_interest' }
                : { field: 'device', op: 'eq', value: 'android' },
            )
          }
        />
      </div>
      {row.tag !== undefined ? (
        <div className="min-w-0 flex-1 basis-36">
          <FormSelect
            label={label + '标签'}
            value={row.tag}
            options={profileTags.map((tag) => ({
              value: tag.id,
              label: tag.label,
            }))}
            disabled={disabled}
            onChange={(tag) => onChange({ tag })}
          />
        </div>
      ) : (
        <>
          <div className="min-w-0 flex-1 basis-28">
            <FormSelect
              label={label + '字段名'}
              value={field}
              options={profileFields.map((field) => ({
                value: field.id,
                label: field.label,
              }))}
              disabled={disabled}
              onChange={(field) =>
                onChange({
                  field,
                  op: 'eq',
                  value:
                    profileFields.find((option) => option.id === field)
                      ?.sample ?? '',
                })
              }
            />
          </div>
          <div className="w-32">
            <FormSelect
              label={label + '比较方式'}
              value={validOp ? row.op! : ''}
              placeholder="请修正比较方式"
              options={allowed.map((op) => ({
                value: op,
                label: opLabels[op],
              }))}
              disabled={disabled}
              onChange={(op) =>
                onChange({ ...row, op: op as Condition['op'], value: '' })
              }
            />
          </div>
          <div className="min-w-0 flex-1 basis-28">
            {dictionary ? (
              <DictionaryValueInput
                label={label + '字典值'}
                value={row.value ?? ''}
                options={dictionary}
                multiple={row.op === 'in'}
                disabled={disabled}
                onChange={(value) => onChange({ ...row, value })}
              />
            ) : (
              <Input
                aria-label={label + '字段值'}
                value={row.value ?? ''}
                disabled={disabled}
                placeholder={
                  row.op === 'in'
                    ? '多个值用英文逗号分隔'
                    : limits
                      ? `${limits.min}～${limits.max}`
                      : '文本值'
                }
                type={limits && row.op !== 'in' ? 'number' : 'text'}
                min={limits?.min}
                max={limits?.max}
                step={limits?.step}
                onChange={(event) =>
                  onChange({ ...row, value: event.target.value })
                }
              />
            )}
          </div>
          {!validOp && (
            <p className="w-full text-xs text-destructive">
              该字段不支持原比较方式，请重新选择。
            </p>
          )}
        </>
      )}
    </>
  );
}
