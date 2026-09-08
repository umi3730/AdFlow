'use client';
import { FormSelect } from '@/components/form-select';
import { Button } from '@/components/ui/button';

export function DictionaryValueInput({
  value,
  options,
  onChange,
  label,
  disabled,
  multiple = false,
}: {
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  label: string;
  disabled?: boolean;
  multiple?: boolean;
}) {
  if (!multiple)
    return (
      <FormSelect
        label={label}
        value={value}
        options={options}
        disabled={disabled}
        onChange={onChange}
      />
    );
  const selected = value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
  const unknown = selected.filter(
    (item) => !options.some((option) => option.value === item),
  );
  function toggle(item: string) {
    onChange(
      selected.includes(item)
        ? selected.filter((current) => current !== item).join(',')
        : [...selected, item].join(','),
    );
  }
  return (
    <fieldset
      aria-label={label}
      disabled={disabled}
      className="flex flex-wrap gap-1.5"
    >
      {options.map((option) => (
        <Button
          type="button"
          key={option.value}
          size="sm"
          variant={selected.includes(option.value) ? 'default' : 'outline'}
          aria-pressed={selected.includes(option.value)}
          disabled={disabled}
          onClick={() => toggle(option.value)}
        >
          {option.label}
        </Button>
      ))}
      {unknown.map((item) => (
        <Button
          type="button"
          key={item}
          size="sm"
          variant="outline"
          className="text-destructive"
          disabled={disabled}
          onClick={() => toggle(item)}
        >
          移除无效值：{item}
        </Button>
      ))}
    </fieldset>
  );
}
