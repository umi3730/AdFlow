'use client';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

export function FormSelect({
  value,
  options,
  onChange,
  label,
  id,
  disabled,
  className,
  placeholder = '请选择',
}: {
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  label: string;
  id?: string;
  disabled?: boolean;
  className?: string;
  placeholder?: string;
}) {
  const known = options.some((option) => option.value === value);
  const items =
    value && !known
      ? [...options, { value, label: '已有值：' + value }]
      : options;
  return (
    <Select
      value={value || null}
      items={items}
      onValueChange={(next) => {
        if (next !== null) onChange(next);
      }}
      disabled={disabled}
    >
      <SelectTrigger
        id={id}
        type="button"
        aria-label={label}
        className={'h-9 w-full min-w-0 bg-background ' + (className ?? '')}
      >
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent
        align="start"
        alignItemWithTrigger={false}
        className="max-h-[min(14rem,var(--available-height))] p-1"
      >
        {items.map((option) => (
          <SelectItem key={option.value} value={option.value} className="py-2">
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
