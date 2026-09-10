'use client';

import { useId } from 'react';
import type { Campaign } from '@/lib/api';
import {
  type DeliveryFilter,
  reportDateRange,
  reportPreset,
} from '@/lib/delivery-report';
import { Input } from '@/components/ui/input';
import { FormSelect } from '@/components/form-select';

export type ReportSelection = {
  start: string;
  end: string;
  granularity: 'hour' | 'day';
  campaignId: string;
};
export function initialReportSelection(
  filter?: DeliveryFilter,
): ReportSelection {
  if (filter)
    return {
      start: new Date(new Date(filter.from).getTime() + 8 * 3600000)
        .toISOString()
        .slice(0, 10),
      end: new Date(new Date(filter.to).getTime() - 1 + 8 * 3600000)
        .toISOString()
        .slice(0, 10),
      granularity: filter.granularity,
      campaignId: filter.campaignId ?? '',
    };
  return { ...reportPreset(7), granularity: 'day', campaignId: '' };
}
export function selectedReportFilter(value: ReportSelection): DeliveryFilter {
  return reportDateRange(
    value.start,
    value.end,
    value.granularity,
    value.campaignId,
  );
}

export function ReportFilters({
  value,
  onChange,
  campaigns,
  disabled = false,
}: {
  value: ReportSelection;
  onChange: (v: ReportSelection) => void;
  campaigns: Campaign[];
  disabled?: boolean;
}) {
  const id = useId();
  return (
    <div className="grid min-w-0 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <label htmlFor={`${id}-start`} className="grid gap-2 text-sm">
        开始日期
        <Input
          id={`${id}-start`}
          aria-label="开始日期"
          type="date"
          value={value.start}
          disabled={disabled}
          onChange={(e) => onChange({ ...value, start: e.target.value })}
        />
      </label>
      <label htmlFor={`${id}-end`} className="grid gap-2 text-sm">
        结束日期
        <Input
          id={`${id}-end`}
          aria-label="结束日期"
          type="date"
          value={value.end}
          disabled={disabled}
          onChange={(e) => onChange({ ...value, end: e.target.value })}
        />
      </label>
      <div className="grid gap-2 text-sm">
        <span>广告计划</span>
        <FormSelect
          label="报表广告计划"
          value={value.campaignId || 'all'}
          disabled={disabled}
          options={[
            { value: 'all', label: '全部计划' },
            ...campaigns.map((c) => ({ value: c.id, label: c.name })),
          ]}
          onChange={(id) =>
            onChange({ ...value, campaignId: id === 'all' ? '' : id })
          }
        />
      </div>
      <div className="grid gap-2 text-sm">
        <span>聚合粒度</span>
        <FormSelect
          label="聚合粒度"
          value={value.granularity}
          disabled={disabled}
          options={[
            { value: 'day', label: '按天' },
            { value: 'hour', label: '按小时' },
          ]}
          onChange={(granularity) =>
            onChange({ ...value, granularity: granularity as 'hour' | 'day' })
          }
        />
      </div>
    </div>
  );
}
