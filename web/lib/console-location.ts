import type { ConsoleView } from '../components/console-guide';
import {
  beijingDate,
  reportDateRange,
  type DeliveryFilter,
} from './delivery-report.ts';

const views: ConsoleView[] = [
  'dashboard',
  'reports',
  'campaigns',
  'creatives',
  'profiles',
  'decision',
  'simulation',
  'operations',
  'agent',
];

export function parseConsoleLocation(search: string) {
  const params = new URLSearchParams(search);
  const requested = params.get('view') as ConsoleView;
  let reportFilter: DeliveryFilter | undefined;
  try {
    const grain = params.get('reportGrain');
    if (grain === 'hour' || grain === 'day')
      reportFilter = reportDateRange(
        params.get('reportStart') ?? '',
        params.get('reportEnd') ?? '',
        grain,
        params.get('reportCampaign') ?? '',
      );
  } catch {
    /* Invalid links use the normal report defaults. */
  }
  return {
    view: views.includes(requested) ? requested : ('dashboard' as ConsoleView),
    query: params.get('q') ?? '',
    slot: params.get('slot') || '__all__',
    status: params.get('status') || '__all__',
    reportFilter,
  };
}

export function reportLocation(filter: DeliveryFilter) {
  return {
    reportStart: beijingDate(new Date(filter.from)),
    reportEnd: beijingDate(new Date(Date.parse(filter.to) - 1)),
    reportGrain: filter.granularity,
    reportCampaign: filter.campaignId ?? '',
  };
}

export function consoleURL(href: string, patch: Record<string, string>) {
  const url = new URL(href);
  for (const [key, value] of Object.entries(patch)) {
    if (
      !value ||
      value === '__all__' ||
      (key === 'view' && value === 'dashboard')
    )
      url.searchParams.delete(key);
    else url.searchParams.set(key, value);
  }
  return url.pathname + url.search + url.hash;
}
