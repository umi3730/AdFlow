'use client';

import {
  CircleDollarSign,
  Gauge,
  MousePointerClick,
  Plus,
  RadioTower,
} from 'lucide-react';
import { Campaign, Metrics } from '@/lib/api';
import { campaignDisplayStatus } from '@/lib/campaign-delivery';
import { Button } from '@/components/ui/button';
import { PageHeading } from '@/components/page-heading';
import { CampaignTable, MetricCard } from './shared';
import type { ConsoleView as View } from '@/components/console-guide';

export function Dashboard({
  now,
  campaigns,
  visibleCampaigns,
  metrics,
  totals,
  connected,
  setView,
  onCreate,
}: {
  now: number | null;
  campaigns: Campaign[];
  visibleCampaigns: Campaign[];
  metrics: Record<string, Metrics>;
  totals: {
    impressions: number;
    clicks: number;
    conversions: number;
    valueFen: number;
  };
  connected: boolean;
  setView: (view: View) => void;
  onCreate?: () => void;
}) {
  const ctr = totals.impressions
    ? `${((totals.clicks / totals.impressions) * 100).toFixed(2)}%`
    : '0.00%';
  return (
    <>
      <PageHeading
        title="投放总览"
        action={
          onCreate && (
            <Button onClick={onCreate}>
              <Plus />
              新建广告计划
            </Button>
          )
        }
      />
      <section className="metric-strip" aria-label="投放指标">
        <MetricCard
          label="投放中"
          value={String(
            campaigns.filter(
              (item) => campaignDisplayStatus(item, now) === 'ACTIVE',
            ).length,
          )}
          change={`${campaigns.length} 个计划`}
          icon={RadioTower}
          tone="mint"
        />
        <MetricCard
          label="累计曝光"
          value={totals.impressions.toLocaleString()}
          change={`${totals.clicks.toLocaleString()} 次点击`}
          icon={Gauge}
          tone="blue"
        />
        <MetricCard
          label="累计点击"
          value={totals.clicks.toLocaleString()}
          change={`CTR ${ctr}`}
          icon={MousePointerClick}
          tone="violet"
        />
        <MetricCard
          label="转化价值"
          value={`¥ ${(totals.valueFen / 100).toFixed(2)}`}
          change={`${totals.conversions} 次转化`}
          icon={CircleDollarSign}
          tone="amber"
        />
      </section>
      <section className="mt-6">
        <CampaignTable
          now={now}
          campaigns={visibleCampaigns.slice(0, 6)}
          metrics={metrics}
          summary={connected ? '累计投放数据' : '服务未连接，数据可能未更新'}
          headerAction={
            <Button variant="ghost" onClick={() => setView('campaigns')}>
              查看全部计划 →
            </Button>
          }
          emptyText={
            campaigns.length ? '没有符合搜索条件的计划' : '还没有广告计划'
          }
          emptyAction={
            campaigns.length === 0 && onCreate ? (
              <Button variant="outline" onClick={onCreate}>
                <Plus />
                创建第一个计划
              </Button>
            ) : undefined
          }
        />
      </section>
    </>
  );
}
