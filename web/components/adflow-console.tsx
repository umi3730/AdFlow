'use client';

import Image from 'next/image';
import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Boxes,
  CheckCircle2,
  CircleDollarSign,
  DatabaseZap,
  Gauge,
  ImageIcon,
  Inbox,
  Layers3,
  LoaderCircle,
  MousePointerClick,
  Plus,
  RadioTower,
  RefreshCw,
  RotateCcw,
  Search,
  Send,
  Sparkles,
  UsersRound,
  X,
} from 'lucide-react';
import {
  api,
  Campaign,
  Creative,
  Decision,
  Metrics,
  KafkaPartitionLag,
  OutboxRecord,
  OutboxStats,
  RuleDraft,
  newClientID,
} from '@/lib/api';
import { useAdFlowTools } from '@/hooks/use-adflow-tools';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

type View =
  | 'dashboard'
  | 'campaigns'
  | 'creatives'
  | 'profiles'
  | 'decision'
  | 'operations'
  | 'agent';

const navItems = [
  { id: 'dashboard' as const, label: '总览', icon: BarChart3 },
  { id: 'campaigns' as const, label: '广告计划', icon: Layers3 },
  { id: 'creatives' as const, label: '素材管理', icon: Boxes },
  { id: 'profiles' as const, label: '用户画像', icon: UsersRound },
  { id: 'decision' as const, label: '决策调试', icon: RadioTower },
  { id: 'operations' as const, label: '运行态', icon: DatabaseZap },
  { id: 'agent' as const, label: 'Agent规则助手', icon: Sparkles },
];

export function AdFlowConsole() {
  const [view, setView] = useState<View>('dashboard');
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [metrics, setMetrics] = useState<Record<string, Metrics>>({});
  const [connected, setConnected] = useState(false);
  const [hasCheckedConnection, setHasCheckedConnection] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [noticeTone, setNoticeTone] = useState<'success' | 'error'>('success');
  const [query, setQuery] = useState('');
  const [lastUpdatedAt, setLastUpdatedAt] = useState<Date | null>(null);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    try {
      const result = await api.listCampaigns();
      setCampaigns(result.items);
      setConnected(true);
      const entries = await Promise.all(
        result.items.map(
          async (campaign) =>
            [campaign.id, await api.metrics(campaign.id)] as const,
        ),
      );
      setMetrics(Object.fromEntries(entries));
      setLastUpdatedAt(new Date());
    } catch {
      setConnected(false);
    } finally {
      setHasCheckedConnection(true);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(refresh);
  }, [refresh]);
  const notify = useCallback((message: string) => {
    setNoticeTone('success');
    setNotice(message);
  }, []);
  useAdFlowTools(refresh, setView, notify);

  async function run(action: () => Promise<void>, message: string) {
    setBusy(true);
    setNotice('');
    try {
      await action();
      setNoticeTone('success');
      setNotice(message);
      await refresh();
    } catch (error) {
      setNoticeTone('error');
      setNotice(error instanceof Error ? error.message : '操作失败');
    } finally {
      setBusy(false);
    }
  }

  const totals = useMemo(
    () =>
      Object.values(metrics).reduce(
        (sum, item) => ({
          impressions: sum.impressions + item.impressions,
          clicks: sum.clicks + item.clicks,
          conversions: sum.conversions + item.conversions,
          valueFen: sum.valueFen + item.valueFen,
        }),
        { impressions: 0, clicks: 0, conversions: 0, valueFen: 0 },
      ),
    [metrics],
  );
  const filteredCampaigns = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase();
    if (!normalized) return campaigns;
    return campaigns.filter((campaign) =>
      [campaign.name, campaign.slotId, campaign.status].some((value) =>
        value.toLocaleLowerCase().includes(normalized),
      ),
    );
  }, [campaigns, query]);
  const searchable = view === 'dashboard' || view === 'campaigns';
  const currentNavItem = navItems.find((item) => item.id === view)!;
  const CurrentNavIcon = currentNavItem.icon;

  return (
    <div className="min-h-screen bg-background text-foreground lg:grid lg:grid-cols-[264px_1fr]">
      <aside className="hidden min-h-screen border-r border-sidebar-border bg-sidebar px-4 py-5 text-sidebar-foreground lg:sticky lg:top-0 lg:flex lg:h-screen lg:flex-col">
        <button
          className="flex items-center gap-3 px-2 text-left"
          onClick={() => setView('dashboard')}
        >
          <div className="grid size-10 place-items-center rounded-xl bg-sidebar-primary text-sidebar-primary-foreground shadow-[0_8px_24px_rgba(39,93,104,.28)]">
            <Gauge className="size-5" />
          </div>
          <div>
            <p className="font-heading text-[17px] font-semibold tracking-[-0.02em]">
              AdFlow
            </p>
            <p className="text-xs text-sidebar-foreground/55">
              Decision Console
            </p>
          </div>
        </button>
        <div className="mx-2 mt-8 text-[10px] font-semibold uppercase tracking-[0.18em] text-sidebar-foreground/35">
          工作区
        </div>
        <nav className="mt-3 space-y-1" aria-label="主导航">
          {navItems.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              onClick={() => setView(id)}
              aria-current={view === id ? 'page' : undefined}
              className={`group flex h-10 w-full items-center gap-3 rounded-lg px-3 text-sm transition ${view === id ? 'bg-sidebar-accent text-sidebar-accent-foreground shadow-[inset_0_0_0_1px_rgba(255,255,255,.04)]' : 'text-sidebar-foreground/62 hover:bg-sidebar-accent/55 hover:text-sidebar-foreground'}`}
            >
              <Icon className="size-4 transition-transform group-hover:scale-105" />
              <span>{label}</span>
              {view === id && (
                <span className="ml-auto size-1.5 rounded-full bg-emerald-400" />
              )}
            </button>
          ))}
        </nav>
        <div className="mt-auto rounded-xl border border-white/8 bg-white/[.04] p-3.5 shadow-[inset_0_1px_0_rgba(255,255,255,.03)]">
          <div
            className={`mb-2 flex items-center gap-2 text-xs font-medium ${connected ? 'text-emerald-300' : 'text-amber-300'}`}
          >
            <Activity className="size-3.5" />
            {connected ? 'Decision Runtime 正常' : '等待本地 API'}
          </div>
          <p className="text-xs leading-5 text-sidebar-foreground/50">
            本地环境 · API :18080
            {lastUpdatedAt && (
              <span className="block">
                {lastUpdatedAt.toLocaleTimeString('zh-CN', {
                  hour: '2-digit',
                  minute: '2-digit',
                })}{' '}
                已同步
              </span>
            )}
          </p>
        </div>
      </aside>

      <main className="min-w-0">
        <header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b bg-background/88 px-4 backdrop-blur-xl sm:px-5 md:px-8">
          <div className="flex items-center gap-3 lg:hidden">
            <button
              type="button"
              className="flex items-center gap-2.5 rounded-lg text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => setView('dashboard')}
              aria-label="返回总览"
            >
              <div className="grid size-9 place-items-center rounded-xl bg-sidebar text-sidebar-primary shadow-[0_8px_20px_rgba(15,53,62,.2)]">
                <Gauge className="size-4" />
              </div>
              <div>
                <p className="text-sm font-semibold leading-4 tracking-[-0.02em]">
                  AdFlow
                </p>
                <p className="mt-0.5 text-[10px] leading-3 text-muted-foreground">
                  {currentNavItem.label}
                </p>
              </div>
            </button>
          </div>
          <div className="hidden min-w-0 flex-1 items-center md:flex">
            {searchable ? (
              <div className="relative w-full max-w-[380px]">
                <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  className="h-9 border-transparent bg-muted/60 pl-9 shadow-none hover:border-border focus-visible:bg-card"
                  placeholder="搜索计划名称、广告位或状态…"
                  aria-label="搜索广告计划"
                />
              </div>
            ) : (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <CurrentNavIcon className="size-4 text-primary" />
                <span>{currentNavItem.label}</span>
              </div>
            )}
          </div>
          <div className="flex items-center gap-2 sm:gap-3">
            <Button
              variant="ghost"
              size="icon-lg"
              onClick={() => void refresh()}
              aria-label="刷新"
              disabled={refreshing}
            >
              <RefreshCw
                className={`size-4 ${refreshing ? 'animate-spin' : ''}`}
              />
            </Button>
            <Badge
              variant="outline"
              className={`gap-1.5 ${
                connected
                  ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
                  : 'border-amber-200 bg-amber-50 text-amber-700'
              }`}
            >
              <span
                className={`size-1.5 rounded-full ${connected ? 'bg-emerald-500' : 'bg-amber-500'}`}
              />
              <span className="hidden sm:inline">
                {connected ? 'API 已连接' : 'API 未连接'}
              </span>
            </Badge>
            <div className="grid size-9 place-items-center rounded-full bg-primary/10 text-xs font-semibold text-primary ring-1 ring-primary/15">
              ZH
            </div>
          </div>
        </header>
        <nav
          className="no-scrollbar sticky top-16 z-10 flex gap-1 overflow-x-auto border-b bg-background/94 px-3 py-2 backdrop-blur-xl lg:hidden"
          aria-label="移动端主导航"
        >
          {navItems.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              onClick={() => setView(id)}
              aria-current={view === id ? 'page' : undefined}
              className={`flex h-9 shrink-0 items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition ${view === id ? 'bg-sidebar text-sidebar-foreground shadow-sm' : 'text-muted-foreground hover:bg-muted hover:text-foreground'}`}
            >
              <Icon
                className={`size-3.5 ${view === id ? 'text-sidebar-primary' : ''}`}
              />
              {label}
            </button>
          ))}
        </nav>
        <div className="mx-auto max-w-[1440px] px-4 py-6 sm:px-5 md:px-8 md:py-8">
          {hasCheckedConnection && !connected && view !== 'dashboard' && (
            <output className="mb-5 flex items-start gap-3 rounded-xl border border-amber-200/80 bg-amber-50/80 px-4 py-3 text-sm text-amber-950 shadow-sm">
              <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600" />
              <div className="min-w-0 flex-1">
                <p className="font-medium">本地 API 暂未连接</p>
                <p className="mt-0.5 text-xs leading-5 text-amber-800/80">
                  页面仍可浏览；启动 :18080
                  后端服务后点击右上角刷新即可恢复实时数据。
                </p>
              </div>
            </output>
          )}
          {notice && (
            <output
              className={`mb-5 flex items-center gap-3 rounded-xl border px-4 py-3 text-sm shadow-sm ${noticeTone === 'error' ? 'border-rose-200 bg-rose-50 text-rose-800' : 'border-emerald-200 bg-emerald-50 text-emerald-800'}`}
              aria-live={noticeTone === 'error' ? 'assertive' : 'polite'}
            >
              {noticeTone === 'error' ? (
                <AlertTriangle className="size-4 shrink-0" />
              ) : (
                <CheckCircle2 className="size-4 shrink-0" />
              )}
              <span className="min-w-0 flex-1">{notice}</span>
              <button
                type="button"
                className="grid size-7 shrink-0 place-items-center rounded-md opacity-60 transition hover:bg-black/5 hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-current"
                onClick={() => setNotice('')}
                aria-label="关闭通知"
              >
                <X className="size-3.5" />
              </button>
            </output>
          )}
          {view === 'dashboard' && (
            <Dashboard
              campaigns={campaigns}
              visibleCampaigns={filteredCampaigns}
              metrics={metrics}
              totals={totals}
              connected={connected}
              setView={setView}
            />
          )}
          {view === 'campaigns' && (
            <CampaignsView
              campaigns={filteredCampaigns}
              metrics={metrics}
              busy={busy}
              run={run}
              query={query}
            />
          )}
          {view === 'creatives' && (
            <CreativesView campaigns={campaigns} busy={busy} run={run} />
          )}
          {view === 'profiles' && <ProfilesView busy={busy} run={run} />}
          {view === 'decision' && <DecisionView busy={busy} run={run} />}
          {view === 'operations' && (
            <OperationsView busy={busy} run={run} connected={connected} />
          )}
          {view === 'agent' && (
            <AgentView campaigns={campaigns} busy={busy} run={run} />
          )}
        </div>
      </main>
    </div>
  );
}

function PageTitle({
  kicker,
  title,
  description,
  action,
}: {
  kicker: string;
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <section className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
      <div>
        <p className="mb-1 text-xs font-semibold uppercase tracking-[0.16em] text-primary/75">
          {kicker}
        </p>
        <h1 className="font-heading text-[28px] font-semibold leading-tight tracking-[-0.035em] md:text-[32px]">
          {title}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">{description}</p>
      </div>
      {action}
    </section>
  );
}

function Dashboard({
  campaigns,
  visibleCampaigns,
  metrics,
  totals,
  connected,
  setView,
}: {
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
}) {
  const ctr = totals.impressions
    ? `${((totals.clicks / totals.impressions) * 100).toFixed(2)}%`
    : '0.00%';
  return (
    <>
      <section className="relative overflow-hidden rounded-[22px] bg-sidebar px-5 py-6 text-sidebar-foreground shadow-[0_22px_60px_rgba(13,49,58,.18)] sm:px-7 sm:py-7">
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -right-16 -top-24 size-72 rounded-full bg-sidebar-primary/12 blur-3xl"
        />
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 opacity-[.06] [background-image:linear-gradient(rgba(255,255,255,.7)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,.7)_1px,transparent_1px)] [background-size:32px_32px]"
        />
        <div className="relative grid gap-6 lg:grid-cols-[1fr_320px] lg:items-end">
          <div>
            <div className="flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.2em] text-sidebar-primary">
              <span className="size-1.5 rounded-full bg-sidebar-primary shadow-[0_0_12px_currentColor]" />
              Real-time delivery
            </div>
            <h1 className="mt-3 max-w-2xl text-[30px] font-semibold leading-[1.08] tracking-[-0.045em] sm:text-[38px]">
              广告决策指挥台
            </h1>
            <p className="mt-3 max-w-xl text-sm leading-6 text-sidebar-foreground/58">
              从计划配置到实时命中，在一个工作台完成投放验证与事件闭环。
            </p>
            <div className="mt-5 flex flex-wrap items-center gap-3">
              <Button
                size="lg"
                className="h-10 bg-sidebar-primary px-4 text-sidebar-primary-foreground hover:bg-sidebar-primary/90"
                onClick={() => setView('campaigns')}
              >
                <Plus />
                新建广告计划
              </Button>
              <div
                className={`flex items-center gap-2 text-xs ${connected ? 'text-emerald-300' : 'text-amber-300'}`}
              >
                <span
                  className={`size-1.5 rounded-full ${connected ? 'bg-emerald-300' : 'bg-amber-300'}`}
                />
                {connected ? '决策服务在线' : '等待本地 API :18080'}
              </div>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-white/10 bg-white/10">
            <div className="bg-sidebar/85 p-4">
              <p className="text-[10px] uppercase tracking-[0.15em] text-sidebar-foreground/40">
                SLA · P99
              </p>
              <p className="mt-2 text-xl font-semibold tracking-[-0.03em] text-white">
                ≤ 80 ms
              </p>
            </div>
            <div className="bg-sidebar/85 p-4">
              <p className="text-[10px] uppercase tracking-[0.15em] text-sidebar-foreground/40">
                CTR
              </p>
              <p className="mt-2 text-xl font-semibold tracking-[-0.03em] text-white tabular-nums">
                {ctr}
              </p>
            </div>
            <div className="col-span-2 flex items-center justify-between bg-sidebar/85 px-4 py-3 text-xs">
              <span className="text-sidebar-foreground/48">当前计划</span>
              <span className="font-medium text-sidebar-foreground">
                {campaigns.filter((item) => item.status === 'ACTIVE').length}{' '}
                投放中 / {campaigns.length} 全部
              </span>
            </div>
          </div>
        </div>
      </section>
      <section className="mt-4 grid grid-cols-2 gap-3 xl:grid-cols-4">
        <MetricCard
          label="投放中"
          value={String(
            campaigns.filter((item) => item.status === 'ACTIVE').length,
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
          campaigns={visibleCampaigns.slice(0, 6)}
          metrics={metrics}
        />
      </section>
    </>
  );
}

function CampaignsView({
  campaigns,
  metrics,
  busy,
  run,
  query,
}: {
  campaigns: Campaign[];
  metrics: Record<string, Metrics>;
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
  query: string;
}) {
  const [name, setName] = useState('');
  const [slotId, setSlotId] = useState('game-home-banner');
  async function create(event: { preventDefault(): void }) {
    event.preventDefault();
    await run(async () => {
      const start = new Date();
      const end = new Date(start.getTime() + 7 * 86400000);
      await api.createCampaign({
        name,
        slotId,
        startAt: start.toISOString(),
        endAt: end.toISOString(),
      });
      setName('');
    }, '广告计划已创建');
  }
  return (
    <>
      <PageTitle
        kicker="计划管理"
        title="广告计划"
        description="配置投放周期、状态和不可变发布版本。"
      />
      <div className="mt-7 grid gap-5 xl:grid-cols-[1fr_360px]">
        <CampaignTable
          campaigns={campaigns}
          metrics={metrics}
          emptyText={query ? `没有匹配“${query}”的计划` : undefined}
          actions={(campaign) => (
            <div className="flex gap-1">
              {campaign.status === 'DRAFT' && (
                <Button
                  size="xs"
                  onClick={() =>
                    void run(
                      () =>
                        api
                          .publishCampaign(campaign.id, {
                            targeting: { all: [{ tag: 'anime' }] },
                            dailyBudgetFen: 100000,
                            impressionCostFen: 100,
                            frequencyLimit: 3,
                          })
                          .then(() => {}),
                      '计划已发布',
                    )
                  }
                >
                  发布示例规则
                </Button>
              )}
              {campaign.status === 'ACTIVE' && (
                <Button
                  size="xs"
                  variant="outline"
                  onClick={() =>
                    void run(
                      () => api.pauseCampaign(campaign.id).then(() => {}),
                      '计划已暂停',
                    )
                  }
                >
                  暂停
                </Button>
              )}
              {campaign.status === 'PAUSED' && (
                <Button
                  size="xs"
                  onClick={() =>
                    void run(
                      () => api.resumeCampaign(campaign.id).then(() => {}),
                      '计划已恢复',
                    )
                  }
                >
                  恢复
                </Button>
              )}
            </div>
          )}
        />
        <Card className="xl:sticky xl:top-24 xl:self-start">
          <CardHeader>
            <CardTitle>创建计划</CardTitle>
            <CardDescription>默认投放7天，创建后为草稿状态。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={create}>
              <Field label="计划名称">
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                  minLength={2}
                  placeholder="例如：策略新游首发"
                />
              </Field>
              <Field label="广告位">
                <Input
                  value={slotId}
                  onChange={(e) => setSlotId(e.target.value)}
                  required
                />
              </Field>
              <Button type="submit" className="w-full" disabled={busy || !name}>
                {busy && <LoaderCircle className="animate-spin" />}
                {busy ? '正在创建…' : '创建草稿'}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function CreativesView({
  campaigns,
  busy,
  run,
}: {
  campaigns: Campaign[];
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
}) {
  const [campaignID, setCampaignID] = useState('');
  const [items, setItems] = useState<Creative[]>([]);
  const [title, setTitle] = useState('');
  const [imageUrl, setImageUrl] = useState(
    'https://images.unsplash.com/photo-1550745165-9bc0b252726f',
  );
  const [landingUrl, setLandingUrl] = useState('https://example.com/game');
  const load = useCallback(async (id: string) => {
    if (!id) {
      setItems([]);
      return;
    }
    try {
      setItems((await api.listCreatives(id)).items);
    } catch {
      setItems([]);
    }
  }, []);
  const selectedCampaignID = campaignID || campaigns[0]?.id || '';
  useEffect(() => {
    void Promise.resolve().then(() => load(selectedCampaignID));
  }, [selectedCampaignID, load]);
  async function create(event: { preventDefault(): void }) {
    event.preventDefault();
    await run(async () => {
      await api.createCreative(selectedCampaignID, {
        title,
        description: '',
        imageUrl,
        landingUrl,
      });
      setTitle('');
      await load(selectedCampaignID);
    }, '素材已创建');
  }
  return (
    <>
      <PageTitle
        kicker="素材资产"
        title="素材管理"
        description="按计划组织广告素材，快速检查状态与投放去向。"
      />
      <div className="mt-7 grid gap-5 xl:grid-cols-[1fr_360px]">
        <Card>
          <CardHeader>
            <CardTitle>素材列表</CardTitle>
            <CardDescription>选择计划后查看关联素材。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <select
              className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm outline-none transition focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
              value={selectedCampaignID}
              onChange={(e) => setCampaignID(e.target.value)}
              aria-label="选择广告计划"
            >
              <option value="">选择广告计划</option>
              {campaigns.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
            {items.length === 0 ? (
              <Empty text="当前计划还没有素材" />
            ) : (
              <div className="grid gap-3 md:grid-cols-2">
                {items.map((item) => (
                  <div
                    key={item.id}
                    className="group overflow-hidden rounded-xl border bg-card transition hover:border-primary/25 hover:shadow-[0_12px_30px_rgba(20,58,66,.08)]"
                  >
                    <div className="relative aspect-[16/7] overflow-hidden bg-muted">
                      <ImageIcon className="absolute left-1/2 top-1/2 size-6 -translate-x-1/2 -translate-y-1/2 text-muted-foreground/35" />
                      <Image
                        src={item.imageUrl}
                        alt={item.title}
                        fill
                        sizes="(min-width: 768px) 32vw, 100vw"
                        unoptimized
                        loader={({ src }) => src}
                        onError={(event) =>
                          event.currentTarget.classList.add('hidden')
                        }
                        className="relative size-full object-cover transition duration-300 group-hover:scale-[1.02]"
                      />
                      <div className="absolute right-2 top-2">
                        <StatusBadge status={item.status} />
                      </div>
                    </div>
                    <div className="p-4">
                      <p className="font-medium">{item.title}</p>
                      <p
                        className="mt-1 truncate text-xs text-muted-foreground"
                        title={item.landingUrl}
                      >
                        {item.landingUrl}
                      </p>
                      {item.status === 'ACTIVE' && (
                        <Button
                          className="mt-4"
                          size="xs"
                          variant="outline"
                          disabled={busy}
                          onClick={() =>
                            void run(async () => {
                              await api.disableCreative(
                                item.campaignId,
                                item.id,
                              );
                              await load(selectedCampaignID);
                            }, '素材已禁用')
                          }
                        >
                          禁用素材
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
        <Card className="xl:sticky xl:top-24 xl:self-start">
          <CardHeader>
            <CardTitle>添加素材</CardTitle>
            <CardDescription>仅允许HTTP或HTTPS资源地址。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={create}>
              <Field label="素材标题">
                <Input
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  required
                  minLength={2}
                />
              </Field>
              <Field label="图片 URL">
                <Input
                  value={imageUrl}
                  onChange={(e) => setImageUrl(e.target.value)}
                  type="url"
                  required
                />
              </Field>
              <Field label="落地页 URL">
                <Input
                  value={landingUrl}
                  onChange={(e) => setLandingUrl(e.target.value)}
                  type="url"
                  required
                />
              </Field>
              <Button
                type="submit"
                className="w-full"
                disabled={busy || !selectedCampaignID}
              >
                {busy && <LoaderCircle className="animate-spin" />}
                {busy ? '正在添加…' : '添加素材'}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function ProfilesView({
  busy,
  run,
}: {
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
}) {
  const [userID, setUserID] = useState('user-10001');
  const [tags, setTags] = useState('anime,strategy_game,active_7d');
  const [device, setDevice] = useState('android');
  const [score, setScore] = useState('88');
  async function save(event: { preventDefault(): void }) {
    event.preventDefault();
    await run(
      () =>
        api.putProfile(userID, {
          tags: tags
            .split(',')
            .map((item) => item.trim())
            .filter(Boolean),
          fields: { device, score },
        }),
      '用户画像已保存',
    );
  }
  return (
    <>
      <PageTitle
        kicker="调试工具"
        title="模拟用户画像"
        description="使用测试标签验证定向规则，不导入任何真实用户数据。"
      />
      <Card className="mt-7 max-w-2xl">
        <CardHeader>
          <CardTitle>画像编辑器</CardTitle>
          <CardDescription>
            标签使用英文逗号分隔，字段用于 eq/gte/lte 条件。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4 md:grid-cols-2" onSubmit={save}>
            <Field label="用户 ID">
              <Input
                value={userID}
                onChange={(e) => setUserID(e.target.value)}
                required
              />
            </Field>
            <Field label="设备">
              <Input
                value={device}
                onChange={(e) => setDevice(e.target.value)}
                required
              />
            </Field>
            <Field label="兴趣标签" className="md:col-span-2">
              <Input value={tags} onChange={(e) => setTags(e.target.value)} />
            </Field>
            <Field label="活跃分数">
              <Input
                value={score}
                onChange={(e) => setScore(e.target.value)}
                inputMode="numeric"
              />
            </Field>
            <div className="flex items-end">
              <Button type="submit" className="w-full" disabled={busy}>
                {busy && <LoaderCircle className="animate-spin" />}
                {busy ? '正在保存…' : '保存测试画像'}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </>
  );
}

function DecisionView({
  busy,
  run,
}: {
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
}) {
  const [userID, setUserID] = useState('user-10001');
  const [slotID, setSlotID] = useState('game-home-banner');
  const [decision, setDecision] = useState<Decision | null>(null);
  async function decide(event: { preventDefault(): void }) {
    event.preventDefault();
    await run(async () => {
      setDecision(
        await api.decide({
          requestId: newClientID('req'),
          userId: userID,
          slotId: slotID,
        }),
      );
    }, '决策已完成');
  }
  async function event(type: 'impression' | 'click' | 'conversion') {
    if (!decision?.matched) return;
    await run(
      () =>
        api
          .recordEvent({
            eventId: newClientID(type),
            requestId: decision.requestId,
            campaignId: decision.campaignId,
            creativeId: decision.creativeId,
            type,
            valueFen: type === 'conversion' ? 500 : undefined,
          })
          .then(() => {}),
      `${type}事件已记录`,
    );
  }
  return (
    <>
      <PageTitle
        kicker="决策链路"
        title="实时决策调试"
        description="调用真实后端，观察命中结果、No-Ad原因与事件闭环。"
      />
      <div className="mt-7 grid gap-5 xl:grid-cols-[420px_1fr]">
        <Card className="bg-[#102d35] text-white">
          <CardHeader>
            <CardTitle className="text-white">发起决策</CardTitle>
            <CardDescription className="text-white/55">
              每次自动生成新的 requestId。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={decide}>
              <Field label="用户 ID" dark>
                <Input
                  className="border-white/10 bg-white/8 text-white"
                  value={userID}
                  onChange={(e) => setUserID(e.target.value)}
                />
              </Field>
              <Field label="广告位" dark>
                <Input
                  className="border-white/10 bg-white/8 text-white"
                  value={slotID}
                  onChange={(e) => setSlotID(e.target.value)}
                />
              </Field>
              <Button
                type="submit"
                className="w-full bg-emerald-300 text-[#102d35] hover:bg-emerald-200"
                disabled={busy}
              >
                {busy ? <LoaderCircle className="animate-spin" /> : <Send />}
                {busy ? '决策执行中…' : '运行决策'}
              </Button>
            </form>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>决策结果</CardTitle>
            <CardDescription>曝光后才允许记录点击和转化。</CardDescription>
          </CardHeader>
          <CardContent>
            {!decision ? (
              <Empty text="填写参数并运行一次决策" />
            ) : (
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <StatusBadge
                    status={decision.matched ? 'MATCHED' : 'NO-AD'}
                  />
                  <span className="text-sm text-muted-foreground">
                    {decision.reason}
                  </span>
                </div>
                <dl className="grid gap-3 text-sm md:grid-cols-2">
                  <Result label="Request" value={decision.requestId} />
                  <Result label="Campaign" value={decision.campaignId || '—'} />
                  <Result label="Creative" value={decision.creativeId || '—'} />
                  <Result
                    label="Expires"
                    value={
                      decision.expiresAt
                        ? new Date(decision.expiresAt).toLocaleTimeString()
                        : '—'
                    }
                  />
                </dl>
                {decision.matched && (
                  <div className="flex flex-wrap gap-2 border-t pt-4">
                    <Button size="sm" onClick={() => void event('impression')}>
                      记录曝光
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => void event('click')}
                    >
                      记录点击
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => void event('conversion')}
                    >
                      记录转化 ¥5
                    </Button>
                  </div>
                )}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function AgentView({
  campaigns,
  busy,
  run,
}: {
  campaigns: Campaign[];
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
}) {
  const [prompt, setPrompt] = useState(
    '面向二次元策略游戏的活跃安卓用户，排除已经安装目标游戏的人，每人每天最多展示3次。',
  );
  const [draft, setDraft] = useState<RuleDraft | null>(null);
  const eligible = campaigns.filter(
    (campaign) => campaign.status === 'DRAFT' || campaign.status === 'PAUSED',
  );
  const [campaignID, setCampaignID] = useState('');
  const selectedCampaignID = campaignID || eligible[0]?.id || '';

  async function generate(event: { preventDefault(): void }) {
    event.preventDefault();
    await run(async () => {
      setDraft(await api.generateRuleDraft(prompt));
    }, '规则草稿已生成，请检查后人工确认');
  }

  async function publish() {
    if (!draft || !selectedCampaignID) return;
    await run(
      () =>
        api
          .publishCampaign(selectedCampaignID, {
            targeting: draft.targeting,
            dailyBudgetFen: draft.dailyBudgetFen,
            impressionCostFen: draft.impressionCostFen,
            frequencyLimit: draft.frequencyLimit,
          })
          .then(() => {}),
      '规则已人工确认并发布为新版本',
    );
  }

  return (
    <>
      <PageTitle
        kicker="AI 辅助配置"
        title="Agent 规则助手"
        description="将自然语言转换成结构化规则草稿；模型没有直接发布权限。"
      />
      <div className="mt-7 grid gap-5 xl:grid-cols-[430px_1fr]">
        <Card className="bg-[#102d35] text-white">
          <CardHeader>
            <div className="mb-2 grid size-10 place-items-center rounded-xl bg-white/10 text-emerald-300">
              <Sparkles className="size-5" />
            </div>
            <CardTitle className="text-white">描述目标人群</CardTitle>
            <CardDescription className="text-white/55">
              当前使用可替换的本地 Mock Provider，输出仍经过领域规则校验。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={generate}>
              <textarea
                value={prompt}
                onChange={(event) => setPrompt(event.target.value)}
                rows={7}
                className="w-full resize-none rounded-lg border border-white/10 bg-white/8 p-3 text-sm leading-6 text-white outline-none placeholder:text-white/35 focus:border-emerald-300/60"
                aria-label="规则描述"
              />
              <Button
                type="submit"
                className="w-full bg-emerald-300 text-[#102d35] hover:bg-emerald-200"
                disabled={busy || prompt.trim().length < 5}
              >
                {busy ? (
                  <LoaderCircle className="animate-spin" />
                ) : (
                  <Sparkles />
                )}
                {busy ? '正在生成…' : '生成结构化草稿'}
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>规则预览与人工确认</CardTitle>
            <CardDescription>
              生成、校验和发布是三个独立步骤，避免模型直接修改线上配置。
            </CardDescription>
          </CardHeader>
          <CardContent>
            {!draft ? (
              <Empty text="输入投放目标并生成第一份规则草稿" />
            ) : (
              <div className="space-y-5">
                <div className="grid gap-3 sm:grid-cols-3">
                  <Result
                    label="日预算"
                    value={`¥ ${(draft.dailyBudgetFen / 100).toFixed(2)}`}
                  />
                  <Result
                    label="单次成本"
                    value={`¥ ${(draft.impressionCostFen / 100).toFixed(2)}`}
                  />
                  <Result
                    label="每日频控"
                    value={`${draft.frequencyLimit} 次`}
                  />
                </div>
                <div className="rounded-lg border bg-muted/35 p-4">
                  <p className="text-sm font-medium">解释</p>
                  <p className="mt-1 text-sm leading-6 text-muted-foreground">
                    {draft.explanation}
                  </p>
                </div>
                <pre className="max-h-72 overflow-auto rounded-lg bg-[#0d2830] p-4 text-xs leading-5 text-emerald-100">
                  {JSON.stringify(draft.targeting, null, 2)}
                </pre>
                {draft.warnings.map((warning) => (
                  <p
                    key={warning}
                    className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs leading-5 text-amber-800"
                  >
                    {warning}
                  </p>
                ))}
                <div className="grid gap-3 border-t pt-4 sm:grid-cols-[1fr_auto]">
                  <select
                    value={selectedCampaignID}
                    onChange={(event) => setCampaignID(event.target.value)}
                    className="h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none transition focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                    aria-label="选择待发布计划"
                  >
                    <option value="">选择草稿或暂停计划</option>
                    {eligible.map((campaign) => (
                      <option key={campaign.id} value={campaign.id}>
                        {campaign.name}
                      </option>
                    ))}
                  </select>
                  <Button
                    disabled={busy || !selectedCampaignID}
                    onClick={() => void publish()}
                  >
                    {busy && <LoaderCircle className="animate-spin" />}
                    {busy ? '正在发布…' : '人工确认并发布'}
                  </Button>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function OperationsView({
  busy,
  run,
  connected,
}: {
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
  connected: boolean;
}) {
  const [status, setStatus] = useState('');
  const [records, setRecords] = useState<OutboxRecord[]>([]);
  const [stats, setStats] = useState<OutboxStats>({
    pending: 0,
    processing: 0,
    published: 0,
    deadLettered: 0,
  });
  const [lag, setLag] = useState<KafkaPartitionLag[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const [outbox, kafka] = await Promise.all([
        api.operationsOutbox(status),
        api.operationsKafkaLag(),
      ]);
      setRecords(outbox.items);
      setStats(outbox.stats);
      setLag(kafka.items);
    } catch (loadError) {
      setError(
        loadError instanceof Error ? loadError.message : '运行态加载失败',
      );
    } finally {
      setLoading(false);
    }
  }, [status]);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const totalLag = lag.reduce((sum, item) => sum + item.lag, 0);
  const unhealthy = stats.pending + stats.processing + stats.deadLettered;
  return (
    <>
      <PageTitle
        kicker="Delivery operations"
        title="运行态工作台"
        description="追踪 Outbox 投递、Kafka 分区积压与死信重放。"
        action={
          <Button
            variant="outline"
            onClick={() => void load()}
            disabled={loading}
          >
            <RefreshCw className={loading ? 'animate-spin' : ''} />
            刷新运行态
          </Button>
        }
      />
      <section className="mt-7 grid grid-cols-2 gap-3 xl:grid-cols-4">
        <MetricCard
          label="待发布"
          value={String(stats.pending)}
          change="Outbox pending"
          icon={Inbox}
          tone="amber"
        />
        <MetricCard
          label="处理中"
          value={String(stats.processing)}
          change="持有 Relay 租约"
          icon={Activity}
          tone="blue"
        />
        <MetricCard
          label="Kafka Lag"
          value={totalLag.toLocaleString()}
          change={`${lag.length} 个分区`}
          icon={RadioTower}
          tone="violet"
        />
        <MetricCard
          label="死信"
          value={String(stats.deadLettered)}
          change={unhealthy === 0 ? '投递链路健康' : '需要运维关注'}
          icon={AlertTriangle}
          tone={stats.deadLettered > 0 ? 'rose' : 'mint'}
        />
      </section>

      <div className="mt-6 grid gap-5 xl:grid-cols-[1fr_360px]">
        <Card className="min-w-0">
          <CardHeader className="border-b">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <CardTitle>Outbox 事件</CardTitle>
                <CardDescription>
                  最近 50 条，死信可由管理员重新入队。
                </CardDescription>
              </div>
              <select
                value={status}
                onChange={(event) => setStatus(event.target.value)}
                className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
              >
                <option value="">全部状态</option>
                <option value="PENDING">待发布</option>
                <option value="PROCESSING">处理中</option>
                <option value="PUBLISHED">已发布</option>
                <option value="DEAD_LETTERED">死信</option>
              </select>
            </div>
          </CardHeader>
          <CardContent className="overflow-x-auto p-0">
            {error ? (
              <Empty text={error} />
            ) : loading ? (
              <Empty text="正在读取投递状态…" />
            ) : records.length === 0 ? (
              <Empty
                text={
                  connected
                    ? '当前筛选下没有 Outbox 事件'
                    : '连接 API 后查看真实运行态'
                }
              />
            ) : (
              <Table className="min-w-[760px]">
                <TableHeader>
                  <TableRow>
                    <TableHead className="pl-5">事件</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>重试</TableHead>
                    <TableHead>时间</TableHead>
                    <TableHead>最近错误</TableHead>
                    <TableHead className="pr-5 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {records.map((record) => (
                    <TableRow key={record.event.eventId}>
                      <TableCell className="pl-5">
                        <p className="font-medium">{record.event.type}</p>
                        <p className="max-w-52 truncate font-mono text-[11px] text-muted-foreground">
                          {record.event.eventId}
                        </p>
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={record.status} />
                      </TableCell>
                      <TableCell className="tabular-nums">
                        {record.attempts}
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                        {new Date(record.createdAt).toLocaleString('zh-CN')}
                      </TableCell>
                      <TableCell
                        className="max-w-64 truncate text-xs text-rose-700"
                        title={record.lastError}
                      >
                        {record.lastError || '—'}
                      </TableCell>
                      <TableCell className="pr-5 text-right">
                        {record.status === 'DEAD_LETTERED' && (
                          <Button
                            size="xs"
                            variant="outline"
                            disabled={busy}
                            onClick={() =>
                              void run(async () => {
                                await api.replayDeadLetter(
                                  record.event.eventId,
                                );
                                await load();
                              }, '死信已重新进入 Outbox 队列')
                            }
                          >
                            <RotateCcw />
                            重放
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        <Card className="self-start bg-[#102d35] text-white xl:sticky xl:top-24">
          <CardHeader>
            <CardTitle className="text-white">Kafka 分区</CardTitle>
            <CardDescription className="text-white/55">
              消费进度由运行中的 Consumer 实时上报。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {lag.length === 0 ? (
              <p className="rounded-lg border border-white/10 bg-white/5 p-4 text-sm text-white/55">
                暂无分区采样；Consumer 收到消息后会显示。
              </p>
            ) : (
              lag.map((item) => (
                <div
                  key={`${item.topic}-${item.partition}`}
                  className="rounded-lg border border-white/10 bg-white/[.05] p-3"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-xs text-white/65">
                      P{item.partition}
                    </span>
                    <span
                      className={
                        item.lag > 0 ? 'text-amber-300' : 'text-emerald-300'
                      }
                    >
                      {item.lag.toLocaleString()} lag
                    </span>
                  </div>
                  <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-white/10">
                    <div
                      className={`h-full rounded-full ${item.lag > 0 ? 'bg-amber-300' : 'bg-emerald-300'}`}
                      style={{
                        width: `${item.lag > 0 ? Math.min(100, 12 + Math.log10(item.lag + 1) * 28) : 4}%`,
                      }}
                    />
                  </div>
                  <p className="mt-2 truncate text-[10px] text-white/35">
                    {item.topic}
                  </p>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function CampaignTable({
  campaigns,
  metrics = {},
  actions,
  emptyText,
}: {
  campaigns: Campaign[];
  metrics?: Record<string, Metrics>;
  actions?: (campaign: Campaign) => React.ReactNode;
  emptyText?: string;
}) {
  return (
    <Card className="border-0 shadow-[0_14px_40px_rgba(20,52,60,.065)] ring-1 ring-foreground/[.07]">
      <CardHeader className="border-b">
        <div className="flex items-center justify-between gap-4">
          <div>
            <CardTitle>广告计划</CardTitle>
            <CardDescription className="mt-0.5">
              {campaigns.length
                ? `${campaigns.length} 个计划 · ${campaigns.filter((item) => item.status === 'ACTIVE').length} 个投放中`
                : '等待创建第一个计划'}
            </CardDescription>
          </div>
          {campaigns.length > 0 && (
            <Badge variant="secondary" className="tabular-nums">
              {campaigns.length}
            </Badge>
          )}
        </div>
      </CardHeader>
      <CardContent className="px-0">
        {campaigns.length === 0 ? (
          <Empty text={emptyText ?? '暂无广告计划，前往广告计划页面创建'} />
        ) : (
          <Table className="min-w-[760px]">
            <TableHeader className="bg-muted/35">
              <TableRow className="hover:bg-transparent">
                <TableHead className="pl-4">计划</TableHead>
                <TableHead>广告位</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">曝光</TableHead>
                <TableHead className="text-right">点击</TableHead>
                <TableHead className="text-right">CTR</TableHead>
                {actions && (
                  <TableHead className="pr-4 text-right">操作</TableHead>
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
                      {campaign.name}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {campaign.slotId}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={campaign.status} />
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
                      <TableCell className="pr-4">
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

function MetricCard({
  label,
  value,
  change,
  icon: Icon,
  tone,
}: {
  label: string;
  value: string;
  change: string;
  icon: typeof Activity;
  tone: 'mint' | 'blue' | 'violet' | 'amber' | 'rose';
}) {
  const toneStyles = {
    mint: 'bg-emerald-50 text-emerald-700 ring-emerald-600/10 dark:bg-emerald-400/10 dark:text-emerald-300',
    blue: 'bg-cyan-50 text-cyan-700 ring-cyan-600/10 dark:bg-cyan-400/10 dark:text-cyan-300',
    violet:
      'bg-violet-50 text-violet-700 ring-violet-600/10 dark:bg-violet-400/10 dark:text-violet-300',
    amber:
      'bg-amber-50 text-amber-700 ring-amber-600/10 dark:bg-amber-400/10 dark:text-amber-300',
    rose: 'bg-rose-50 text-rose-700 ring-rose-600/10 dark:bg-rose-400/10 dark:text-rose-300',
  };
  return (
    <Card className="gap-2 border-0 py-4 shadow-[0_10px_28px_rgba(20,52,60,.055)] ring-1 ring-foreground/[.065] transition duration-200 hover:-translate-y-0.5 hover:shadow-[0_16px_36px_rgba(20,52,60,.085)]">
      <CardHeader className="grid grid-cols-[1fr_auto] items-start px-4">
        <div>
          <CardDescription className="text-xs">{label}</CardDescription>
          <CardTitle className="mt-1.5 text-xl font-semibold tracking-[-0.035em] tabular-nums sm:text-2xl">
            {value}
          </CardTitle>
        </div>
        <div
          className={`grid size-8 place-items-center rounded-lg ring-1 ${toneStyles[tone]}`}
        >
          <Icon className="size-4" />
        </div>
      </CardHeader>
      <CardContent className="px-4">
        <span className="text-[11px] text-muted-foreground">{change}</span>
      </CardContent>
    </Card>
  );
}
function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
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
    PROCESSING: 'border-cyan-200 bg-cyan-50 text-cyan-700',
    PUBLISHED: 'border-emerald-200 bg-emerald-50 text-emerald-700',
    DEAD_LETTERED: 'border-rose-200 bg-rose-50 text-rose-700',
  };
  const labels: Record<string, string> = {
    ACTIVE: '投放中',
    MATCHED: '已命中',
    PAUSED: '已暂停',
    DRAFT: '草稿',
    DISABLED: '已禁用',
    'NO-AD': '未命中',
    PENDING: '待发布',
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
function Field({
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
      className={`block space-y-1.5 text-xs font-medium ${dark ? 'text-white/70' : 'text-muted-foreground'} ${className}`}
    >
      <span className="block">{label}</span>
      {children}
    </label>
  );
}
function Empty({ text }: { text: string }) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-3 px-6 text-center text-sm text-muted-foreground">
      <div className="grid size-10 place-items-center rounded-xl bg-muted text-muted-foreground/70">
        <Inbox className="size-4" />
      </div>
      <p>{text}</p>
    </div>
  );
}
function Result({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-lg bg-muted/55 p-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-all font-mono text-xs leading-5">{value}</dd>
    </div>
  );
}
