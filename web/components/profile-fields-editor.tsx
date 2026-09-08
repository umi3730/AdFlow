'use client';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Trash2 } from 'lucide-react';
import {
  profileFields,
  fieldValueOptions,
  numericFieldLimits,
} from '@/lib/profile-options';
import { DictionaryValueInput } from '@/components/dictionary-value-input';

export function ProfileFieldsEditor({
  value,
  onChange,
  disabled,
}: {
  value: Record<string, string>;
  onChange: (value: Record<string, string>) => void;
  disabled?: boolean;
}) {
  const extraFields = profileFields.filter(
    (field) => field.id !== 'device' && field.id !== 'score',
  );
  const legacy = Object.entries(value).filter(
    ([key]) => !profileFields.some((field) => field.id === key),
  );
  function change(key: string, text: string) {
    const next = { ...value };
    if (text) next[key] = text;
    else delete next[key];
    onChange(next);
  }
  return (
    <fieldset disabled={disabled} className="space-y-3">
      {extraFields.map((field) => (
        <label key={field.id} className="block space-y-1 text-sm">
          {field.label}
          {fieldValueOptions(field.id) ? (
            <DictionaryValueInput
              label={field.label}
              value={value[field.id] ?? ''}
              options={fieldValueOptions(field.id)!}
              disabled={disabled}
              onChange={(text) => change(field.id, text)}
            />
          ) : (
            <Input
              value={value[field.id] ?? ''}
              placeholder={field.sample}
              type={numericFieldLimits(field.id) ? 'number' : 'text'}
              min={numericFieldLimits(field.id)?.min}
              max={numericFieldLimits(field.id)?.max}
              step={numericFieldLimits(field.id)?.step}
              onChange={(event) => change(field.id, event.target.value)}
            />
          )}
        </label>
      ))}
      {legacy.length > 0 && (
        <details className="rounded-lg border p-3 text-sm">
          <summary className="cursor-pointer text-muted-foreground">
            已有扩展字段（兼容保留）
          </summary>
          <div className="mt-3 space-y-3">
            {legacy.map(([key, current]) => (
              <div key={key} className="flex items-end gap-2">
                <label className="min-w-0 flex-1 space-y-1">
                  {key}
                  <Input
                    value={current}
                    onChange={(event) => change(key, event.target.value)}
                  />
                </label>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  aria-label={'移除字段' + key}
                  onClick={() => change(key, '')}
                >
                  <Trash2 />
                </Button>
              </div>
            ))}
          </div>
        </details>
      )}
    </fieldset>
  );
}
