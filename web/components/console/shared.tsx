'use client';

import { Activity, Inbox } from 'lucide-react';
import { Campaign, Metrics } from '@/lib/api';
import {
  campaignDisplayStatus,
  formatCampaignDate,
} from '@/lib/campaign-delivery';
import { adSlotLabel } from '@/lib/ad-slots';
import { Badge } from '@/components/ui/badge';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

export function CampaignTable({
  now,
  campaigns,
  metrics = {},
  actions,
  emptyText,
  onInspect,
  toolbar,
  summary,
  hideTitle = false,
  headerAction,
  emptyAction,
}: {
  now: number | null;
  campaigns: Campaign[];
  metrics?: Record<string, Metrics>;
  actions?: (campaign: Campaign) => React.ReactNode;
  emptyText?: string;
  onInspect?: (campaign: Campaign) => void;
  toolbar?: React.ReactNode;
  summary?: string;
  hideTitle?: boolean;
  headerAction?: React.ReactNode;
  emptyAction?: React.ReactNode;
}) {
  return (
    <Card className="min-w-0 gap-0 pb-0">
      <CardHeader className="border-b">
        <div className="flex items-center justify-between gap-4">
          <div>
            {!hideTitle && <CardTitle>广告计划</CardTitle>}
            <CardDescription className="mt-0.5">
              {summary ??
                (campaigns.length
                  ? `${campaigns.length} 个计划 · ${campaigns.filter((item) => campaignDisplayStatus(item, now) === 'ACTIVE').length} 个投放中`
                  : '等待创建第一个计划')}
            </CardDescription>
          </div>
          {headerAction}
        </div>
      </CardHeader>
      <CardContent className="px-0">
        {toolbar && <div className="border-b p-4">{toolbar}</div>}
        {campaigns.length === 0 ? (
          <Empty text={emptyText ?? '还没有广告计划'} action={emptyAction} />
        ) : (
          <Table className="min-w-[760px]">
            <TableHeader className="bg-muted/35">
              <TableRow className="hover:bg-transparent">
                <TableHead className="pl-4">计划</TableHead>
                <TableHead>广告位</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>投放时间（北京时间）</TableHead>
                <TableHead className="text-right">曝光</TableHead>
                <TableHead className="text-right">点击</TableHead>
                <TableHead className="text-right">CTR</TableHead>
                {actions && (
                  <TableHead className="sticky right-0 z-10 bg-card pr-4 text-right shadow-[-5px_0_8px_-7px_rgba(0,0,0,.3)]">
                    操作
                  </TableHead>
                )}
              </TableRow>
            </TableHeader>
            <TableBody>
              {campaigns.map((campaign) => {
                const performance = metrics[campaign.id];
                const impressions = performance?.impressions ?? 0;
                const clicks = performance?.clicks ?? 0;
                const ctr = impressions
                  ? `${((clicks / impressions) * 100).toFixed(2)}%`
                  : '—';
                return (
                  <TableRow key={campaign.id}>
                    <TableCell className="pl-4 font-medium">
                      {onInspect ? (
                        <button
                          type="button"
                          className="text-left text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={() => onInspect(campaign)}
                          aria-label={`查看 ${campaign.name} 的规则`}
                        >
                          {campaign.name}
                        </button>
                      ) : (
                        campaign.name
                      )}
                      {campaign.activeVersion?.auction && (
                        <p className="mt-1 text-xs text-muted-foreground">
                          {campaign.activeVersion.auction.advertiserName} · 出价
                          ¥
                          {(
                            campaign.activeVersion.auction.bidFen / 100
                          ).toFixed(2)}
                        </p>
                      )}
                      {campaign.activeVersion && (
                        <span className="ml-2 text-xs text-muted-foreground">
                          v{campaign.activeVersion.number}
                        </span>
                      )}
                    </TableCell>
                    <TableCell
                      className="text-sm text-muted-foreground"
                      title={campaign.slotId}
                    >
                      {adSlotLabel(campaign.slotId)}
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        status={campaignDisplayStatus(campaign, now)}
                      />
                    </TableCell>
                    <TableCell className="text-xs leading-5 text-muted-foreground tabular-nums">
                      <div>起 {formatCampaignDate(campaign.startAt)}</div>
                      <div>止 {formatCampaignDate(campaign.endAt)}</div>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {impressions.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {clicks.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right font-medium tabular-nums">
                      {ctr}
                    </TableCell>
                    {actions && (
                      <TableCell className="sticky right-0 z-10 bg-card pr-4 shadow-[-5px_0_8px_-7px_rgba(0,0,0,.3)]">
                        <div className="flex justify-end">
                          {actions(campaign)}
                        </div>
                      </TableCell>
                    )}
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

export function MetricCard({
  label,
  value,
  change,
  tone,
}: {
  label: string;
  value: string;
  change: string;
  icon: typeof Activity;
  tone: 'mint' | 'blue' | 'violet' | 'amber' | 'rose';
}) {
  return (
    <div>
      <p className="text-sm text-muted-foreground">{label}</p>
      <p
        className={
          'mt-2 break-words text-[26px] font-semibold leading-tight tracking-tight tabular-nums sm:text-3xl ' +
          (tone === 'rose' ? 'text-destructive' : 'text-foreground')
        }
      >
        {value}
      </p>
      <p className="mt-2 text-xs leading-5 text-muted-foreground">{change}</p>
    </div>
  );
}

export function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    NEEDS_CREATIVE: 'border-amber-200 bg-amber-50 text-amber-800',
    SCHEDULED: 'border-blue-200 bg-blue-50 text-blue-700',
    ENDED: 'border-slate-200 bg-slate-50 text-slate-600',
    INVALID_PERIOD: 'border-rose-200 bg-rose-50 text-rose-700',
    CHECKING: 'border-slate-200 text-muted-foreground',
    ACTIVE:
      'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-400/25 dark:bg-emerald-400/10 dark:text-emerald-300',
    MATCHED:
      'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-400/25 dark:bg-emerald-400/10 dark:text-emerald-300',
    PAUSED:
      'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-400/25 dark:bg-amber-400/10 dark:text-amber-300',
    DRAFT:
      'border-slate-200 bg-slate-50 text-slate-600 dark:border-slate-400/20 dark:bg-slate-400/10 dark:text-slate-300',
    DISABLED:
      'border-slate-200 bg-slate-50 text-slate-500 dark:border-slate-400/20 dark:bg-slate-400/10 dark:text-slate-400',
    'NO-AD':
      'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-400/25 dark:bg-rose-400/10 dark:text-rose-300',
    PENDING: 'border-amber-200 bg-amber-50 text-amber-700',
    SETTLING: 'border-amber-200 bg-amber-50 text-amber-700',
    RECONCILE: 'border-rose-200 bg-rose-50 text-rose-700',
    PROCESSING: 'border-cyan-200 bg-cyan-50 text-cyan-700',
    PUBLISHED: 'border-emerald-200 bg-emerald-50 text-emerald-700',
    DEAD_LETTERED: 'border-rose-200 bg-rose-50 text-rose-700',
  };
  const labels: Record<string, string> = {
    NEEDS_CREATIVE: '待添加素材',
    SCHEDULED: '待开始',
    ENDED: '已结束',
    INVALID_PERIOD: '时间异常',
    CHECKING: '检查中',
    ACTIVE: '投放中',
    MATCHED: '已命中',
    PAUSED: '已暂停',
    DRAFT: '草稿',
    DISABLED: '已禁用',
    'NO-AD': '未命中',
    PENDING: '待发布',
    SETTLING: '等待结算',
    RECONCILE: '待核对',
    PROCESSING: '处理中',
    PUBLISHED: '已发布',
    DEAD_LETTERED: '死信',
  };
  return (
    <Badge variant="outline" className={styles[status] ?? ''} title={status}>
      {labels[status] ?? status}
    </Badge>
  );
}

export function Field({
  label,
  children,
  className = '',
  dark = false,
}: {
  label: string;
  children: React.ReactNode;
  className?: string;
  dark?: boolean;
}) {
  return (
    <label
      className={`block space-y-1.5 text-sm font-medium ${dark ? 'text-foreground' : 'text-muted-foreground'} ${className}`}
    >
      <span className="block">{label}</span>
      {children}
    </label>
  );
}

export function Empty({
  text,
  action,
}: {
  text: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex min-h-48 flex-col items-center justify-center gap-3 px-6 text-center text-sm text-muted-foreground">
      <Inbox className="size-5 text-muted-foreground/60" />
      <p>{text}</p>
      {action}
    </div>
  );
}

export function Result({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-lg bg-muted/55 p-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-all font-mono text-xs leading-5">{value}</dd>
    </div>
  );
}
