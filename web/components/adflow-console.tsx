'use client';

import Image from 'next/image';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Boxes,
  CheckCircle2,
  CircleDollarSign,
  DatabaseZap,
  Gauge,
  CircleHelp,
  FlaskConical,
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
  Profile,
  TargetingExplanation,
  newClientID,
  simulationAPIBase,
  type TraceTarget,
} from '@/lib/api';
import { useCampaignClock } from '@/hooks/use-campaign-clock';
import {
  campaignDisplayStatus,
  formatCampaignDate,
} from '@/lib/campaign-delivery';
import { useAdFlowTools } from '@/hooks/use-adflow-tools';
import { CampaignRuleDialog } from '@/components/campaign-rule-dialog';
import { ProfileWorkspace } from '@/components/profile-workspace';
import { UserPoolSimulation } from '@/components/user-pool-simulation';
import { RequestTraceDialog } from '@/components/request-trace-dialog';
import { ConsoleHelp, type ConsoleView } from '@/components/console-guide';
import { AgentWorkspace } from '@/components/agent-workspace';
import {
  CreativeAssetPicker,
  CreativeDropZone,
} from '@/components/creative-asset-picker';
import { DecisionPricing } from '@/components/decision-pricing';
import { CreativeImage } from '@/components/creative-image';
import { FormSelect } from '@/components/form-select';
import { useAccess, SessionIdentity, RoleGate } from '@/components/auth-gate';
import {
  allCampaignFilters,
  campaignStatusOptions,
  filterCampaigns,
} from '@/lib/campaign-filters';
import { adSlotOptions, defaultAdSlotID, adSlotLabel } from '@/lib/ad-slots';
import { localCreativeImageURL, isDemoCreative } from '@/lib/demo-creatives';
import { DeleteResourceButton } from '@/components/delete-resource-button';
import {
  nextTestCreativeNumber,
  selectedCreativeCampaignID,
  testCreativeTitle,
} from '@/lib/creative-defaults';
import {
  newAgentCampaignInput,
  nextTestPlanNumber,
  testPlanName,
} from '@/lib/agent-campaign';
import type { RuleEditorValue } from '@/lib/campaign-rules';
import {
  canRecordDecisionEvent,
  createDecisionEventController,
  type DecisionEventType,
} from '@/lib/decision-events';
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
import { profileTagLabel } from '@/lib/profile-options';
import { PageHeading } from '@/components/page-heading';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

const apiEndpointHost = (() => {
  try {
    return new URL(simulationAPIBase).host;
  } catch {
    return '未配置';
  }
})();

type View = ConsoleView;

const navItems = [
  { id: 'dashboard' as const, label: '总览', icon: BarChart3 },
  { id: 'campaigns' as const, label: '广告计划', icon: Layers3 },
  { id: 'creatives' as const, label: '素材管理', icon: Boxes },
  { id: 'profiles' as const, label: '用户画像', icon: UsersRound },
  { id: 'decision' as const, label: '单次投放测试', icon: RadioTower },
  { id: 'simulation' as const, label: '批量投放测试', icon: FlaskConical },
  { id: 'operations' as const, label: '事件处理', icon: DatabaseZap },
  { id: 'agent' as const, label: 'Agent规则助手', icon: Sparkles },
];

export function AdFlowConsole() {
  const { canOperate, canAdmin } = useAccess();
  const visibleNav = navItems.filter((item) =>
    item.id === 'simulation'
      ? canAdmin
      : ['agent', 'decision'].includes(item.id)
        ? canOperate
        : true,
  );
  const [view, setView] = useState<View>('dashboard');
  const [helpOpen, setHelpOpen] = useState(false);
  const [traceTarget, setTraceTarget] = useState<TraceTarget | null>(null);
  const inspectRequest = useCallback(
    (target: TraceTarget) => setTraceTarget(target),
    [setTraceTarget],
  );
  const [demoRequest, setDemoRequest] = useState(0);
  const [newPlanOpen, setNewPlanOpen] = useState(false);
  const [simulationRunning, setSimulationRunning] = useState(false);
  const [simulationRefreshUntil, setSimulationRefreshUntil] = useState(0);
  const wasSimulationRunning = useRef(false);
  const handleSimulationRunningChange = useCallback((running: boolean) => {
    if (wasSimulationRunning.current && !running)
      setSimulationRefreshUntil(Date.now() + 15000);
    wasSimulationRunning.current = running;
    setSimulationRunning(running);
  }, []);
  const [simulationStopSignal, setSimulationStopSignal] = useState(0);
  const refreshTask = useRef<Promise<void> | null>(null);
  const refreshQueued = useRef(false);
  const [decisionUserID, setDecisionUserID] = useState('');
  const [pendingAgentDraft, setPendingAgentDraft] =
    useState<RuleEditorValue | null>(null);
  const [campaignDrafts, setCampaignDrafts] = useState<
    Record<string, RuleEditorValue>
  >({});
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const now = useCampaignClock(campaigns);
  const [metrics, setMetrics] = useState<Record<string, Metrics>>({});
  const [connected, setConnected] = useState(false);
  const [hasCheckedConnection, setHasCheckedConnection] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [noticeTone, setNoticeTone] = useState<'success' | 'error'>('success');
  const [query, setQuery] = useState('');
  const [lastUpdatedAt, setLastUpdatedAt] = useState<Date | null>(null);

  const refresh = useCallback(() => {
    if (refreshTask.current) {
      refreshQueued.current = true;
      return refreshTask.current;
    }
    const task = (async () => {
      setRefreshing(true);
      do {
        refreshQueued.current = false;
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
        }
      } while (refreshQueued.current);
    })().finally(() => {
      refreshTask.current = null;
      setRefreshing(false);
    });
    refreshTask.current = task;
    return task;
  }, []);

  useEffect(() => {
    void Promise.resolve().then(refresh);
  }, [refresh]);
  useEffect(() => {
    if (
      (!simulationRunning && Date.now() >= simulationRefreshUntil) ||
      (view !== 'dashboard' && view !== 'campaigns')
    )
      return;
    let live = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      if (!live) return;
      await refresh();
      if (live && (simulationRunning || Date.now() < simulationRefreshUntil))
        timer = setTimeout(poll, 1000);
    };
    void poll();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [simulationRunning, simulationRefreshUntil, view, refresh]);
  const testSequence = nextTestPlanNumber(campaigns);
  function campaignCreated(campaign: Campaign) {
    if (pendingAgentDraft) {
      setCampaignDrafts((previous) => ({
        ...previous,
        [campaign.id]: structuredClone(pendingAgentDraft),
      }));
      setPendingAgentDraft(null);
    }
    setCampaigns((previous) => [
      ...previous.filter((item) => item.id !== campaign.id),
      campaign,
    ]);
  }
  const notify = useCallback(
    (message: string) => {
      setNoticeTone('success');
      setNotice(message);
    },
    [setNotice, setNoticeTone],
  );
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
  const filteredCampaigns = useMemo(
    () =>
      filterCampaigns(
        campaigns,
        query,
        allCampaignFilters,
        allCampaignFilters,
        now,
      ),
    [campaigns, query, now],
  );
  const searchable = view === 'dashboard';
  const currentNavItem = navItems.find((item) => item.id === view)!;
  const CurrentNavIcon = currentNavItem.icon;

  return (
    <div className="min-h-screen bg-background text-foreground lg:grid lg:grid-cols-[224px_minmax(0,1fr)]">
      <aside className="hidden min-h-screen border-r border-sidebar-border bg-sidebar px-3 py-5 text-sidebar-foreground lg:sticky lg:top-0 lg:flex lg:h-screen lg:flex-col">
        <button
          type="button"
          aria-label="AdFlow · 返回总览"
          className="mx-3 w-fit rounded-sm py-1 text-left outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-4 focus-visible:ring-offset-sidebar"
          onClick={() => setView('dashboard')}
        >
          <span className="block font-heading text-[26px] font-semibold leading-none tracking-[-0.045em]">
            Ad<span className="text-sidebar-primary">Flow</span>
          </span>
          <span className="mt-2.5 block text-xs tracking-[0.08em] text-sidebar-foreground/65">
            广告决策平台
          </span>
        </button>
        <nav className="mt-8 space-y-1" aria-label="主导航">
          {visibleNav.map(({ id, label, icon: Icon }) => (
            <div key={id}>
              {['campaigns', 'decision', 'operations'].includes(id) && (
                <p className="px-3 pb-2 pt-4 text-xs font-medium tracking-wide text-sidebar-foreground/60">
                  {
                    {
                      campaigns: '投放配置',
                      decision: '投放验证',
                      operations: '系统与辅助',
                    }[id as 'campaigns' | 'decision' | 'operations']
                  }
                </p>
              )}
              <button
                type="button"
                onClick={() => setView(id)}
                aria-current={view === id ? 'page' : undefined}
                className={`group flex h-10 w-full items-center gap-3 rounded-md px-3 text-sm transition-colors outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring ${view === id ? 'bg-sidebar-primary text-sidebar-primary-foreground font-medium' : 'text-sidebar-foreground/80 hover:bg-sidebar-accent/55 hover:text-sidebar-foreground'}`}
              >
                <Icon className="size-4" />
                <span>{label}</span>
              </button>
            </div>
          ))}
        </nav>
        <div className="mt-auto border-t border-sidebar-border px-3 pt-4">
          <div
            className={`mb-2 flex items-center gap-2 text-xs font-medium ${connected ? 'text-emerald-200' : 'text-amber-200'}`}
          >
            <Activity className="size-3.5" />
            {connected ? 'Decision Runtime 正常' : '等待本地 API'}
          </div>
          <p className="text-xs leading-5 text-sidebar-foreground/75">
            本地环境 · API {apiEndpointHost}
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
        <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b bg-background px-4 sm:px-5 md:px-8">
          <div className="flex items-center gap-3 lg:hidden">
            <button
              type="button"
              className="flex items-center gap-2.5 rounded-lg text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => setView('dashboard')}
              aria-label="返回总览"
            >
              <div>
                <p className="font-heading text-lg font-semibold leading-5 tracking-[-0.045em]">
                  Ad<span className="text-secondary-foreground">Flow</span>
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
              aria-label="功能说明"
              onClick={() => setHelpOpen(true)}
            >
              <CircleHelp className="size-4" />
            </Button>
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
              aria-label={connected ? 'API 已连接' : 'API 未连接'}
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
            <SessionIdentity />
          </div>
        </header>
        <nav
          className="no-scrollbar sticky top-14 z-10 flex gap-1 overflow-x-auto border-b bg-background px-3 py-2 lg:hidden"
          aria-label="移动端主导航"
        >
          {visibleNav.map(({ id, label, icon: Icon }) => (
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
        <div className="mx-auto max-w-[1600px] px-4 py-6 sm:px-5 md:px-8">
          {simulationRunning && view !== 'simulation' && (
            <div className="mb-5 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-primary/20 bg-primary/5 px-4 py-3 text-sm">
              <span>批量投放测试运行中</span>
              <div className="flex gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setView('simulation')}
                >
                  查看模拟
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setSimulationStopSignal((value) => value + 1)}
                >
                  停止模拟
                </Button>
              </div>
            </div>
          )}
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
              now={now}
              campaigns={campaigns}
              visibleCampaigns={filteredCampaigns}
              metrics={metrics}
              totals={totals}
              connected={connected}
              setView={setView}
              onCreate={
                canOperate
                  ? () => {
                      setNewPlanOpen(true);
                      setView('campaigns');
                    }
                  : undefined
              }
            />
          )}
          {view === 'campaigns' && (
            <CampaignsView
              now={now}
              newPlanOpen={newPlanOpen}
              onNewPlanOpenChange={setNewPlanOpen}
              campaigns={campaigns}
              metrics={metrics}
              busy={busy || refreshing || !hasCheckedConnection}
              run={run}
              query={query}
              onQueryChange={setQuery}
              onChanged={refresh}
              defaultName={testPlanName(testSequence)}
              agentDraft={pendingAgentDraft}
              campaignDrafts={campaignDrafts}
              onCreated={campaignCreated}
              onDiscardAgentDraft={() => setPendingAgentDraft(null)}
              onPublished={(campaign) =>
                setCampaignDrafts((previous) => {
                  const next = { ...previous };
                  delete next[campaign.id];
                  return next;
                })
              }
            />
          )}
          {view === 'creatives' && (
            <CreativesView campaigns={campaigns} busy={busy} run={run} />
          )}
          {view === 'profiles' && (
            <ProfileWorkspace
              onDecide={(userID) => {
                setDecisionUserID(userID);
                setView('decision');
              }}
            />
          )}
          {view === 'decision' && (
            <RoleGate minimum="operator">
              <DecisionView
                onInspectRequest={inspectRequest}
                busy={busy}
                run={run}
                initialUserID={decisionUserID}
                campaigns={campaigns}
              />
            </RoleGate>
          )}
          {view === 'operations' && (
            <OperationsView
              busy={busy}
              run={run}
              connected={connected}
              onInspectRequest={inspectRequest}
            />
          )}
          <div hidden={view !== 'simulation'}>
            <RoleGate minimum="admin">
              <UserPoolSimulation
                onInspectRequest={inspectRequest}
                demoRequest={demoRequest}
                now={now}
                active={view === 'simulation'}
                campaigns={campaigns}
                onFinished={refresh}
                onRunningChange={handleSimulationRunningChange}
                stopSignal={simulationStopSignal}
              />
            </RoleGate>
          </div>
          {view === 'agent' && (
            <RoleGate minimum="operator">
              <AgentWorkspace
                onCreatePlan={(value) => {
                  setPendingAgentDraft(structuredClone(value));
                  setQuery('');
                  setNotice('');
                  setNewPlanOpen(true);
                  setView('campaigns');
                }}
              />
            </RoleGate>
          )}
          <ConsoleHelp
            open={helpOpen}
            onOpenChange={setHelpOpen}
            onNavigate={setView}
            allowedViews={visibleNav.map((item) => item.id)}
            demoDisabled={simulationRunning || busy || !connected}
            onPrepareDemo={() => {
              setHelpOpen(false);
              setDemoRequest((value) => value + 1);
              setView('simulation');
            }}
          />
          <RequestTraceDialog
            target={traceTarget}
            onClose={() => setTraceTarget(null)}
          />
        </div>
      </main>
    </div>
  );
}

function Dashboard({
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

function CampaignsView({
  now,
  newPlanOpen,
  onNewPlanOpenChange,
  campaigns,
  metrics,
  busy,
  run,
  query,
  onQueryChange,
  onChanged,
  defaultName,
  agentDraft,
  campaignDrafts,
  onCreated,
  onDiscardAgentDraft,
  onPublished,
}: {
  now: number | null;
  newPlanOpen: boolean;
  onNewPlanOpenChange: (open: boolean) => void;
  campaigns: Campaign[];
  metrics: Record<string, Metrics>;
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
  query: string;
  onQueryChange: (value: string) => void;
  onChanged: () => Promise<void>;
  defaultName: string;
  agentDraft: RuleEditorValue | null;
  campaignDrafts: Record<string, RuleEditorValue>;
  onCreated: (campaign: Campaign) => void;
  onDiscardAgentDraft: () => void;
  onPublished: (campaign: Campaign) => void;
}) {
  const { canOperate, canAdmin } = useAccess();
  const [nameOverride, setName] = useState<string | null>(null);
  const [createError, setCreateError] = useState('');
  const createTriggerRef = useRef<HTMLButtonElement>(null);
  const [filterSlot, setFilterSlot] = useState(allCampaignFilters);
  const [filterStatus, setFilterStatus] = useState(allCampaignFilters);
  const hasFilters =
    Boolean(query.trim()) ||
    filterSlot !== allCampaignFilters ||
    filterStatus !== allCampaignFilters;
  const visibleCampaigns = filterCampaigns(
    campaigns,
    query,
    filterSlot,
    filterStatus,
    now,
  );
  function resetFilters() {
    onQueryChange('');
    setFilterSlot(allCampaignFilters);
    setFilterStatus(allCampaignFilters);
  }
  const name = nameOverride ?? defaultName;
  const creating = useRef(false);
  const createNameRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (agentDraft && newPlanOpen) createNameRef.current?.focus();
  }, [agentDraft, newPlanOpen]);
  const [slotId, setSlotId] = useState(defaultAdSlotID);
  const [selectedCampaign, setSelectedCampaign] = useState<Campaign | null>(
    null,
  );
  async function create(event: { preventDefault(): void }) {
    event.preventDefault();
    if (creating.current || busy) return;
    creating.current = true;
    setCreateError('');
    try {
      await run(async () => {
        let created: Campaign;
        try {
          created = await api.createCampaign(
            newAgentCampaignInput(name, slotId),
          );
        } catch (error) {
          setCreateError(
            error instanceof Error ? error.message : '创建失败，请重试',
          );
          throw error;
        }
        onCreated(created);
        setName(null);
        onNewPlanOpenChange(false);
        if (agentDraft) setSelectedCampaign(created);
      }, '计划草稿已创建，尚未投放');
    } finally {
      creating.current = false;
    }
  }
  return (
    <>
      <PageHeading
        title="广告计划"
        action={
          canOperate && (
            <Button
              ref={createTriggerRef}
              onClick={() => {
                setCreateError('');
                onNewPlanOpenChange(true);
              }}
            >
              <Plus />
              新建计划
            </Button>
          )
        }
      />
      <div className="mt-5">
        <CampaignTable
          now={now}
          campaigns={visibleCampaigns}
          metrics={metrics}
          emptyText={
            hasFilters
              ? '没有符合筛选条件的计划，试试调整条件或重置'
              : '还没有广告计划'
          }
          hideTitle
          emptyAction={
            !hasFilters && canOperate ? (
              <Button
                variant="outline"
                onClick={() => onNewPlanOpenChange(true)}
              >
                <Plus />
                创建第一个计划
              </Button>
            ) : undefined
          }
          summary={`显示 ${visibleCampaigns.length} / ${campaigns.length} 个计划 · ${visibleCampaigns.filter((item) => campaignDisplayStatus(item, now) === 'ACTIVE').length} 个投放中`}
          toolbar={
            <div className="grid min-w-0 gap-2 sm:grid-cols-2 xl:grid-cols-[minmax(200px,1fr)_200px_160px_auto]">
              <Input
                aria-label="搜索计划"
                placeholder="搜索计划名称或广告位"
                value={query}
                onChange={(event) => onQueryChange(event.target.value)}
                className="min-w-0"
              />
              <FormSelect
                label="筛选广告位"
                value={filterSlot}
                onChange={setFilterSlot}
                options={[
                  { value: allCampaignFilters, label: '全部广告位' },
                  ...adSlotOptions,
                ]}
              />
              <FormSelect
                label="筛选计划状态"
                value={filterStatus}
                onChange={setFilterStatus}
                options={campaignStatusOptions}
              />
              <Button
                type="button"
                variant="outline"
                disabled={!hasFilters}
                onClick={resetFilters}
              >
                <RotateCcw />
                重置
              </Button>
            </div>
          }
          onInspect={setSelectedCampaign}
          actions={(campaign) => (
            <div className="flex gap-1">
              <Button
                type="button"
                size="sm"
                variant={campaign.status === 'DRAFT' ? 'default' : 'outline'}
                disabled={busy}
                onClick={() => setSelectedCampaign(campaign)}
              >
                {canAdmin &&
                (campaign.status === 'DRAFT' || campaign.status === 'PAUSED')
                  ? '编辑规则'
                  : '查看规则'}
              </Button>
              {campaign.status === 'ACTIVE' && (
                <Button
                  size="xs"
                  variant="outline"
                  disabled={busy || !canOperate}
                  onClick={() =>
                    void run(
                      () => api.pauseCampaign(campaign.id).then(() => {}),
                      '计划已暂停',
                    )
                  }
                >
                  {campaignDisplayStatus(campaign, now) === 'ENDED'
                    ? '停用'
                    : '暂停'}
                </Button>
              )}
              {campaign.status === 'PAUSED' && (
                <Button
                  size="xs"
                  disabled={
                    busy ||
                    !canOperate ||
                    ['ENDED', 'INVALID_PERIOD', 'CHECKING'].includes(
                      campaignDisplayStatus(campaign, now),
                    )
                  }
                  title={
                    campaignDisplayStatus(campaign, now) === 'ENDED'
                      ? '投放期已结束，请新建计划'
                      : undefined
                  }
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
              <DeleteResourceButton
                name={campaign.name}
                description="计划将从列表移除，历史版本和统计保留。投放中的计划需先暂停；本次不提供恢复入口。"
                disabled={busy || campaign.status === 'ACTIVE'}
                disabledReason={
                  campaign.status === 'ACTIVE' ? '先暂停再删除' : undefined
                }
                onDelete={() => api.deleteCampaign(campaign.id)}
                onDeleted={async () => {
                  if (selectedCampaign?.id === campaign.id)
                    setSelectedCampaign(null);
                  onPublished(campaign);
                  await onChanged();
                }}
              />
            </div>
          )}
        />
        <Dialog
          open={newPlanOpen && canOperate}
          onOpenChange={(open) => {
            if (!busy) onNewPlanOpenChange(open);
          }}
        >
          <DialogContent
            className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-md"
            initialFocus={createNameRef}
            finalFocus={createTriggerRef}
          >
            <DialogHeader>
              <DialogTitle>新建广告计划</DialogTitle>
              <DialogDescription>
                默认 7 天，确认发布后才投放。
              </DialogDescription>
            </DialogHeader>
            <form className="space-y-4" onSubmit={create}>
              {createError && (
                <p
                  role="alert"
                  className="rounded-md bg-destructive/10 p-3 text-sm text-destructive"
                >
                  {createError}
                </p>
              )}
              {agentDraft && (
                <div className="space-y-2 rounded-lg bg-primary/5 p-3 text-sm">
                  <p className="font-medium">Agent 规则已带入</p>
                  <p className="text-muted-foreground">
                    日预算 ¥{agentDraft.dailyBudgetYuan} · 单次 ¥
                    {agentDraft.impressionCostYuan} · 每人每天{' '}
                    {agentDraft.frequencyLimit} 次
                  </p>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={onDiscardAgentDraft}
                  >
                    取消带入
                  </Button>
                </div>
              )}
              <Field label="计划名称">
                <Input
                  ref={createNameRef}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                  minLength={2}
                  maxLength={128}
                  disabled={busy}
                  placeholder="例如：策略新游首发"
                />
              </Field>
              <Field label="广告位">
                <FormSelect
                  label="广告位"
                  value={slotId}
                  options={adSlotOptions}
                  onChange={setSlotId}
                  disabled={busy}
                />
              </Field>
              <Button
                type="submit"
                className="w-full"
                disabled={busy || !name.trim()}
              >
                {busy && <LoaderCircle className="animate-spin" />}
                {busy
                  ? '正在创建…'
                  : agentDraft
                    ? '创建草稿并确认规则'
                    : '创建草稿'}
              </Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>
      {selectedCampaign && (
        <CampaignRuleDialog
          now={now}
          key={selectedCampaign.id}
          campaign={selectedCampaign}
          initialValue={
            selectedCampaign.status === 'DRAFT' &&
            !selectedCampaign.activeVersion
              ? campaignDrafts[selectedCampaign.id]
              : undefined
          }
          onClose={() => setSelectedCampaign(null)}
          onChanged={onChanged}
          onPublished={onPublished}
        />
      )}
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
  const { canOperate } = useAccess();
  const [campaignID, setCampaignID] = useState('');
  const [items, setItems] = useState<Creative[]>([]);
  const [loadingCreatives, setLoadingCreatives] = useState(false);
  const [creativeLoadError, setCreativeLoadError] = useState('');
  const creativeRequest = useRef(0);
  const cancelCreativeLoad = useCallback(() => {
    creativeRequest.current++;
  }, []);
  const [titleOverride, setTitle] = useState<string | null>(null);
  const sequence = nextTestCreativeNumber(items);
  const title = titleOverride ?? testCreativeTitle(sequence);
  const creating = useRef(false);
  const [assetID, setAssetID] = useState('');
  const assetLibraryRef = useRef<HTMLDivElement>(null);
  const [landingUrl, setLandingUrl] = useState('https://example.com/game');
  const load = useCallback(async (id: string) => {
    const request = ++creativeRequest.current;
    setCreativeLoadError('');
    if (!id) {
      setItems([]);
      setLoadingCreatives(false);
      return;
    }
    setLoadingCreatives(true);
    try {
      const result = await api.listCreatives(id);
      if (request === creativeRequest.current) setItems(result.items);
    } catch (cause) {
      if (request === creativeRequest.current) {
        setItems([]);
        setCreativeLoadError(
          cause instanceof Error ? cause.message : '素材列表读取失败',
        );
      }
    } finally {
      if (request === creativeRequest.current) setLoadingCreatives(false);
    }
  }, []);
  const selectedCampaignID = selectedCreativeCampaignID(campaigns, campaignID);
  useEffect(() => {
    let live = true;
    void Promise.resolve().then(() => {
      if (live) void load(selectedCampaignID);
    });
    return () => {
      live = false;
      cancelCreativeLoad();
    };
  }, [selectedCampaignID, load, cancelCreativeLoad]);
  async function create(event: { preventDefault(): void }) {
    event.preventDefault();
    if (
      creating.current ||
      busy ||
      !canOperate ||
      !selectedCampaignID ||
      !assetID
    )
      return;
    if (loadingCreatives || creativeLoadError) return;
    creating.current = true;
    try {
      await run(async () => {
        const created = await api.createCreative(selectedCampaignID, {
          title,
          description: '',
          imageUrl: localCreativeImageURL(assetID, window.location.origin),
          landingUrl,
        });
        setItems((previous) => [
          ...previous.filter((item) => item.id !== created.id),
          created,
        ]);
        setTitle(null);
        setAssetID('');
        await load(selectedCampaignID);
      }, '素材已创建');
    } finally {
      creating.current = false;
    }
  }
  return (
    <>
      <PageHeading
        title="素材管理"
        description="按计划组织广告素材，快速检查状态与投放去向。"
      />
      <div className="mt-5 grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <Card>
          <CardHeader>
            <CardTitle>素材列表</CardTitle>
            <CardDescription>选择计划后查看关联素材。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <FormSelect
              label="选择广告计划"
              placeholder="请选择广告计划"
              value={selectedCampaignID || '__no_campaign__'}
              disabled={busy}
              options={[
                { value: '__no_campaign__', label: '不选择计划' },
                ...campaigns.map((item) => ({
                  value: item.id,
                  label: item.name,
                })),
              ]}
              onChange={(id) => {
                const nextID = id === '__no_campaign__' ? '' : id;
                if (nextID === campaignID) return;
                cancelCreativeLoad();
                setCampaignID(nextID);
                setAssetID('');
                setItems([]);
                setCreativeLoadError('');
                setLoadingCreatives(Boolean(nextID));
              }}
            />
            {creativeLoadError && (
              <div
                role="alert"
                className="flex flex-wrap items-center gap-2 rounded-lg bg-destructive/5 p-3 text-sm text-destructive"
              >
                {creativeLoadError}
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => void load(selectedCampaignID)}
                >
                  重试
                </Button>
              </div>
            )}
            <form
              className="space-y-4 rounded-xl border p-4"
              onSubmit={create}
              aria-label="左侧添加素材"
            >
              <CreativeDropZone
                key={`${selectedCampaignID}:${assetID || 'empty-selection'}`}
                value={assetID}
                onChange={setAssetID}
                disabled={busy}
                onBrowse={() => {
                  const target =
                    assetLibraryRef.current?.querySelector<HTMLButtonElement>(
                      '[aria-pressed="true"]',
                    ) ??
                    assetLibraryRef.current?.querySelector<HTMLButtonElement>(
                      'button',
                    );
                  target?.focus();
                }}
              />
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="素材标题">
                  <Input
                    value={title}
                    disabled={busy}
                    maxLength={128}
                    onChange={(event) => setTitle(event.target.value)}
                    required
                    minLength={2}
                  />
                </Field>
                <Field label="落地页 URL">
                  <Input
                    value={landingUrl}
                    disabled={busy}
                    onChange={(event) => setLandingUrl(event.target.value)}
                    type="url"
                    required
                  />
                </Field>
              </div>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-xs text-muted-foreground">
                  {selectedCampaignID && assetID
                    ? '确认后加入当前计划的素材列表'
                    : !selectedCampaignID
                      ? '请先选择广告计划'
                      : '从右侧拖入或点击选择下一份素材'}
                </span>
                <Button
                  type="submit"
                  disabled={
                    busy ||
                    !canOperate ||
                    !selectedCampaignID ||
                    !assetID ||
                    loadingCreatives ||
                    Boolean(creativeLoadError)
                  }
                >
                  {busy && <LoaderCircle className="animate-spin" />}
                  {busy ? '正在添加…' : '确认添加素材'}
                </Button>
              </div>
            </form>
            {loadingCreatives ? (
              <output className="block py-6 text-center text-sm text-muted-foreground">
                正在读取当前计划的素材…
              </output>
            ) : items.length === 0 ? (
              <Empty
                text={
                  selectedCampaignID ? '当前计划还没有素材' : '请先选择广告计划'
                }
              />
            ) : (
              <div className="grid gap-3 md:grid-cols-2">
                {items.map((item) => (
                  <div
                    key={item.id}
                    className="group overflow-hidden rounded-lg border bg-card transition-colors hover:border-primary/40"
                  >
                    <div className="relative flex h-48 items-center justify-center overflow-hidden bg-muted">
                      {!isDemoCreative(item.imageUrl) && (
                        <ImageIcon className="absolute left-1/2 top-1/2 size-6 -translate-x-1/2 -translate-y-1/2 text-muted-foreground/35" />
                      )}
                      {isDemoCreative(item.imageUrl) ? (
                        <div className="relative">
                          <CreativeImage
                            src={item.imageUrl}
                            alt={item.title}
                            size={160}
                          />
                        </div>
                      ) : (
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
                          style={{ objectFit: 'contain' }}
                          className="p-4"
                        />
                      )}
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
                          disabled={busy || !canOperate}
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
                      {item.status === 'DISABLED' && (
                        <Button
                          type="button"
                          className="mt-4"
                          size="xs"
                          disabled={busy || !canOperate}
                          onClick={() =>
                            void run(async () => {
                              await api.enableCreative(
                                item.campaignId,
                                item.id,
                              );
                              await load(selectedCampaignID);
                            }, '素材已启用；候选快照默认约 5 秒内刷新')
                          }
                        >
                          重新启用
                        </Button>
                      )}
                      <DeleteResourceButton
                        name={item.title}
                        description="素材将从列表移除，历史投放记录保留。请先禁用素材；删除后不能重新启用。"
                        disabled={busy || item.status === 'ACTIVE'}
                        disabledReason={
                          item.status === 'ACTIVE' ? '先禁用再删除' : undefined
                        }
                        onDelete={() =>
                          api.deleteCreative(item.campaignId, item.id)
                        }
                        onDeleted={() => load(selectedCampaignID)}
                      />
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
        <Card className="xl:sticky xl:top-24 xl:self-start">
          <CardHeader>
            <CardTitle>本地素材库</CardTitle>
            <CardDescription>拖到左侧接收区，或点击选用。</CardDescription>
          </CardHeader>
          <CardContent>
            <CreativeAssetPicker
              value={assetID}
              onChange={setAssetID}
              disabled={busy}
              libraryRef={assetLibraryRef}
            />
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function DecisionView({
  busy,
  run,
  initialUserID,
  campaigns,
  onInspectRequest,
}: {
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
  initialUserID: string;
  campaigns: Campaign[];
  onInspectRequest: (target: TraceTarget) => void;
}) {
  const [userID, setUserID] = useState(initialUserID);
  const [slotID, setSlotID] = useState(defaultAdSlotID);
  const [decision, setDecision] = useState<Decision | null>(null);
  const [eventController] = useState(() =>
    createDecisionEventController(newClientID),
  );
  const [eventState, setEventState] = useState(() =>
    eventController.snapshot(),
  );
  const deciding = useRef(false);
  const decisionRevision = useRef(0);
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [profileError, setProfileError] = useState('');
  const [explanation, setExplanation] = useState<TargetingExplanation | null>(
    null,
  );
  const [explanationError, setExplanationError] = useState('');
  useEffect(
    () => () => {
      decisionRevision.current++;
      eventController.select(null);
    },
    [eventController],
  );
  useEffect(() => {
    let live = true;
    void api
      .listProfiles({ limit: 100 })
      .then((page) => {
        if (live) setProfiles(page.items);
      })
      .catch((cause) => {
        if (live)
          setProfileError(
            cause instanceof Error ? cause.message : '用户列表读取失败',
          );
      });
    return () => {
      live = false;
    };
  }, []);
  useEffect(() => {
    let live = true;
    const timer = setTimeout(() => {
      if (!userID.trim()) return;
      void api
        .getProfile(userID.trim())
        .then((result) => {
          if (live) {
            setProfile(result);
            setProfileError('');
          }
        })
        .catch((cause) => {
          if (live) {
            setProfile(null);
            setProfileError(
              cause instanceof Error ? cause.message : '画像读取失败',
            );
          }
        });
    }, 200);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [userID]);

  function clearDecision() {
    decisionRevision.current++;
    eventController.select(null);
    setEventState(eventController.snapshot());
    setDecision(null);
    setExplanation(null);
    setExplanationError('');
  }
  function chooseUser(id: string) {
    setUserID(id);
    setProfile(null);
    setProfileError('');
    clearDecision();
  }
  async function decide(event: { preventDefault(): void }) {
    event.preventDefault();
    if (busy || deciding.current || eventController.isPending()) return;
    deciding.current = true;
    clearDecision();
    const revision = decisionRevision.current;
    try {
      await run(async () => {
        const result = await api.decide({
          requestId: newClientID('req'),
          userId: userID.trim(),
          slotId: slotID.trim(),
        });
        if (revision !== decisionRevision.current) return;
        setDecision(result);
        eventController.select(result);
        setEventState(eventController.snapshot());
        try {
          const report = await api.explainDecision({
            userId: userID.trim(),
            slotId: slotID.trim(),
          });
          if (revision !== decisionRevision.current) return;
          setExplanation(report);
          setProfile(report.profile);
        } catch (cause) {
          if (revision === decisionRevision.current)
            setExplanationError(
              cause instanceof Error ? cause.message : '定向检查暂不可用',
            );
        }
      }, '决策已完成');
    } finally {
      deciding.current = false;
    }
  }
  async function event(type: DecisionEventType) {
    if (busy || deciding.current) return;
    const submission = eventController.begin(type);
    if (!submission) return;
    setEventState(eventController.snapshot());
    await run(
      async () => {
        try {
          await api.recordEvent(submission);
          if (eventController.finish(submission, true))
            setEventState(eventController.snapshot());
        } catch (cause) {
          if (eventController.finish(submission, false))
            setEventState(eventController.snapshot());
          throw cause;
        }
      },
      `${{ impression: '曝光', click: '点击', conversion: '转化' }[type]}事件已受理`,
    );
  }
  return (
    <>
      <PageHeading
        title="单次投放测试"
        description="选择一个用户，查看广告选择结果，再回传曝光、点击或转化。"
      />
      <div className="mt-5 grid items-start gap-5 xl:grid-cols-[minmax(280px,360px)_minmax(0,1fr)]">
        <Card className="bg-card text-card-foreground">
          <CardHeader>
            <CardTitle>发起决策</CardTitle>
            <CardDescription>每次自动生成新的 requestId。</CardDescription>
          </CardHeader>
          <CardContent>
            <form className="space-y-4" onSubmit={decide}>
              <Field label="选择已保存用户" dark>
                <select
                  aria-label="选择已保存用户"
                  className="h-9 w-full rounded-lg border border-input bg-card px-2 text-sm text-foreground"
                  value={
                    profiles.some((item) => item.userId === userID)
                      ? userID
                      : ''
                  }
                  disabled={busy}
                  onChange={(event) => chooseUser(event.target.value)}
                >
                  <option value="">选择用户（或在下方输入 ID）</option>
                  {profiles.map((item) => (
                    <option key={item.userId} value={item.userId}>
                      {item.userId}
                    </option>
                  ))}
                </select>
                <p className="mt-1 text-xs text-muted-foreground">
                  快捷列表最多显示 100 人；更多用户可在画像表格中筛选。
                </p>
              </Field>
              <Field label="用户 ID" dark>
                <Input
                  className="bg-card text-foreground"
                  value={userID}
                  onChange={(e) => chooseUser(e.target.value)}
                  required
                  disabled={busy}
                />
              </Field>
              <Field label="广告位" dark>
                <FormSelect
                  label="广告位"
                  className="bg-card text-foreground"
                  value={slotID}
                  options={adSlotOptions}
                  onChange={(value) => {
                    setSlotID(value);
                    clearDecision();
                  }}
                  disabled={busy}
                />
              </Field>
              {profileError && (
                <p
                  role="alert"
                  className="rounded-lg bg-amber-50 p-3 text-sm text-amber-800"
                >
                  {profileError}
                </p>
              )}
              {profile && profile.userId === userID.trim() && (
                <div className="space-y-2 rounded-lg border bg-card p-3">
                  <p className="text-sm font-medium">
                    已保存画像 · {profile.userId}
                  </p>
                  <div className="flex flex-wrap gap-1">
                    {profile.tags.map((tag) => (
                      <Badge
                        key={tag}
                        variant="outline"
                        className="border-border bg-secondary text-secondary-foreground"
                      >
                        {profileTagLabel(tag)}
                      </Badge>
                    ))}
                  </div>
                  <dl className="space-y-1 text-sm">
                    {Object.entries(profile.fields).map(([key, value]) => (
                      <div
                        key={key}
                        className="flex flex-wrap justify-between gap-2"
                      >
                        <dt className="text-muted-foreground">{key}</dt>
                        <dd className="break-all">{value}</dd>
                      </div>
                    ))}
                  </dl>
                </div>
              )}
              <Button
                type="submit"
                className="w-full"
                disabled={busy || !userID.trim() || !slotID.trim()}
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
            <CardDescription>
              曝光需在决策有效期内回传；曝光受理后，点击和转化可在 7
              天内回传。异步统计稍后更新。
            </CardDescription>
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
                    {decisionReasonLabel(decision.reason)}
                  </span>
                </div>
                {decision.matched && (
                  <DecisionPricing pricing={decision.pricing} />
                )}
                <dl className="grid gap-3 text-sm md:grid-cols-2">
                  <div>
                    <Result label="Request" value={decision.requestId} />
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      className="mt-2"
                      onClick={() =>
                        onInspectRequest({ requestId: decision.requestId })
                      }
                    >
                      查看处理过程
                    </Button>
                  </div>
                  <Result label="Campaign" value={decision.campaignId || '—'} />
                  <Result label="Creative" value={decision.creativeId || '—'} />
                  <Result
                    label="曝光回传截止"
                    value={
                      decision.expiresAt
                        ? new Date(decision.expiresAt).toLocaleTimeString()
                        : '—'
                    }
                  />
                </dl>
                {explanationError && (
                  <p className="rounded-lg bg-amber-50 p-3 text-sm text-amber-800">
                    定向检查：{explanationError}
                  </p>
                )}
                {explanation && (
                  <TargetingReport report={explanation} campaigns={campaigns} />
                )}
                {decision.matched && (
                  <div className="flex flex-wrap gap-2 border-t pt-4">
                    <Button
                      size="sm"
                      disabled={
                        busy ||
                        !canRecordDecisionEvent(eventState, 'impression')
                      }
                      onClick={() => void event('impression')}
                    >
                      {eventState.impression === 'recorded'
                        ? '曝光已受理'
                        : eventState.impression === 'pending'
                          ? '曝光记录中…'
                          : '记录曝光'}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={
                        busy || !canRecordDecisionEvent(eventState, 'click')
                      }
                      title={
                        eventState.impression !== 'recorded'
                          ? '请先记录曝光'
                          : undefined
                      }
                      onClick={() => void event('click')}
                    >
                      {eventState.click === 'recorded'
                        ? '点击已受理'
                        : eventState.click === 'pending'
                          ? '点击记录中…'
                          : '记录点击'}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={
                        busy ||
                        !canRecordDecisionEvent(eventState, 'conversion')
                      }
                      title={
                        eventState.impression !== 'recorded'
                          ? '请先记录曝光'
                          : undefined
                      }
                      onClick={() => void event('conversion')}
                    >
                      {eventState.conversion === 'recorded'
                        ? '转化已受理 ¥5'
                        : eventState.conversion === 'pending'
                          ? '转化记录中…'
                          : '记录转化 ¥5'}
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

function decisionReasonLabel(reason: string) {
  const labels: Record<string, string> = {
    matched: '已命中广告',
    profile_not_found: '用户画像不存在',
    no_candidate: '该广告位暂无有效投放计划',
    targeting_miss: '用户未满足定向条件',
    frequency_capped: '该用户已达到每日曝光上限',
    budget_exhausted: '预算不足或已被预占',
    dependency_unavailable: '依赖服务暂不可用',
  };
  return labels[reason] ?? reason;
}

function TargetingReport({
  report,
  campaigns,
}: {
  report: TargetingExplanation;
  campaigns: Campaign[];
}) {
  const groupLabels = {
    all: '必须全部满足',
    any: '至少满足一条（当前均不满足）',
    none: '命中排除条件',
  };
  return (
    <section className="space-y-3 border-t pt-4" aria-label="当前定向检查">
      <h3 className="font-semibold">当前定向检查</h3>
      <p className="text-sm text-muted-foreground">
        用户 {report.profile.userId} ·{' '}
        {new Date(report.checkedAt).toLocaleTimeString('zh-CN')}
        。仅检查当前标签、字段和素材，不占预算/频控，也不是历史决策审计。
      </p>
      {report.candidates.length === 0 && (
        <p className="rounded-lg bg-muted/50 p-3 text-sm">
          该广告位当前没有有效候选计划。检查广告位是否一致、计划是否发布、是否在投放周期内。
        </p>
      )}
      {report.candidates.map((candidate) => (
        <div
          key={candidate.campaignId}
          className="space-y-2 rounded-lg border p-3"
        >
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="font-medium">
              {campaigns.find((item) => item.id === candidate.campaignId)
                ?.name ?? candidate.campaignId}
            </p>
            <Badge
              variant="outline"
              className={
                candidate.targetingMatched
                  ? 'text-emerald-700'
                  : 'text-rose-700'
              }
            >
              {candidate.targetingMatched ? '定向条件满足' : '定向条件不满足'}
            </Badge>
          </div>
          {!candidate.hasCreative && (
            <p className="text-sm text-amber-700">
              该计划没有有效素材，即使定向满足也不能投放。
            </p>
          )}
          {candidate.failures.map((failure, index) => {
            const condition = failure.condition;
            let message = '';
            if (failure.code === 'invalid_condition')
              message = `规则字段 ${condition.field} 的比较方式或值不合法，请修正规则后重新发布。`;
            else if (failure.code === 'missing_tag')
              message = `缺少标签：${condition.tag}`;
            else if (failure.code === 'missing_field')
              message = `缺少字段：${condition.field}；规则要求 ${condition.op} ${condition.value}`;
            else if (failure.code === 'excluded_condition')
              message = condition.tag
                ? `用户带有排除标签：${condition.tag}`
                : `命中排除字段：${condition.field} ${condition.op} ${condition.value}`;
            else
              message = `${condition.field} 的实际值是「${failure.actual ?? ''}」，规则要求 ${condition.op}「${condition.value}」`;
            return (
              <div
                key={index}
                className="rounded-lg bg-rose-50 p-3 text-sm text-rose-900"
              >
                <p className="mb-1 text-xs font-semibold">
                  {groupLabels[failure.group]}
                </p>
                <p className="break-words">{message}</p>
                {condition.field === 'platform' &&
                  report.profile.fields.device !== undefined && (
                    <p className="mt-1">
                      画像已有 device={report.profile.fields.device}，但
                      platform 是不同字段；请在规则编辑器中确认并转换。
                    </p>
                  )}
              </div>
            );
          })}
          {candidate.targetingMatched && candidate.hasCreative && (
            <p className="text-sm text-muted-foreground">
              标签和字段检查通过；最终是否投放仍受频控、预算和素材选择影响。
            </p>
          )}
        </div>
      ))}
    </section>
  );
}

function OperationsView({
  busy,
  run,
  connected,
  onInspectRequest,
}: {
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
  connected: boolean;
  onInspectRequest: (target: TraceTarget) => void;
}) {
  const { canAdmin } = useAccess();
  const [inspectId, setInspectId] = useState('');
  const traceLookup = (
    <form
      className="my-5 flex flex-wrap items-end gap-3 rounded-lg border bg-card p-4"
      onSubmit={(event) => {
        event.preventDefault();
        if (inspectId.trim()) onInspectRequest({ requestId: inspectId.trim() });
      }}
    >
      <label
        htmlFor="trace-request-id"
        className="min-w-0 flex-1 space-y-2 text-sm"
      >
        按请求查看处理过程
        <Input
          id="trace-request-id"
          aria-label="查询请求 ID"
          placeholder="粘贴测试结果中的 requestId"
          value={inspectId}
          maxLength={128}
          onChange={(event) => setInspectId(event.target.value)}
        />
      </label>
      <Button type="submit" variant="outline" disabled={!inspectId.trim()}>
        查询请求
      </Button>
    </form>
  );
  const [runtimeMode, setRuntimeMode] = useState<Awaited<
    ReturnType<typeof api.operationsMode>
  > | null>(null);
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
    setRuntimeMode(null);
    try {
      const mode = await api.operationsMode();
      setRuntimeMode(mode);
      if (!mode.outboxEnabled) {
        setRecords([]);
        setLag([]);
        setStats({ pending: 0, processing: 0, published: 0, deadLettered: 0 });
        return;
      }
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
  const unhealthy =
    stats.pending +
    stats.processing +
    stats.deadLettered +
    (stats.settling ?? 0) +
    (stats.reconcile ?? 0);
  const available = runtimeMode?.outboxEnabled && !loading && !error;
  if (runtimeMode?.eventTransport === 'sync' && !loading)
    return (
      <>
        <PageHeading
          title="事件处理"
          description="当前事件处理模式由后端配置决定。"
        />
        {traceLookup}
        <Card className="mt-7">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <DatabaseZap className="size-5 text-primary" />
              同步事件模式
            </CardTitle>
            <CardDescription>
              未启用 MySQL Outbox 与 Kafka 异步链路，不展示积压或死信指标。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">
              曝光、点击和转化当前同步计入内存统计，可在总览查看。这里没有消息记录不等于
              Kafka 链路健康；切换为 Kafka 模式后才会显示投递和消费状态。
            </p>
            <Button type="button" variant="outline" onClick={() => void load()}>
              <RefreshCw />
              重新检查运行模式
            </Button>
          </CardContent>
        </Card>
      </>
    );
  return (
    <>
      <PageHeading
        title="事件处理"
        description={
          loading
            ? '正在读取运行模式…'
            : error
              ? '运行状态不可用，请重试。'
              : 'Kafka 异步模式 · 追踪 Outbox 投递、分区积压与死信重放。'
        }
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
      {traceLookup}
      <section className="metric-strip">
        <MetricCard
          label="待发布"
          value={available ? String(stats.pending) : '—'}
          change="Outbox pending"
          icon={Inbox}
          tone="amber"
        />
        <MetricCard
          label="处理中"
          value={available ? String(stats.processing) : '—'}
          change="持有 Relay 租约"
          icon={Activity}
          tone="blue"
        />
        <MetricCard
          label="Kafka Lag"
          value={available && lag.length ? totalLag.toLocaleString() : '—'}
          change={
            available && lag.length
              ? `${lag.length} 个分区`
              : '尚无有效分区采样'
          }
          icon={RadioTower}
          tone="violet"
        />
        <MetricCard
          label="死信"
          value={available ? String(stats.deadLettered) : '—'}
          change={
            !available
              ? '尚未获得有效状态'
              : unhealthy === 0
                ? '当前无待处理或死信'
                : '需要运维关注'
          }
          icon={AlertTriangle}
          tone={stats.deadLettered > 0 ? 'rose' : 'mint'}
        />
      </section>

      <div className="mt-6 grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <Card className="min-w-0">
          <CardHeader className="border-b">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <CardTitle>Outbox 事件</CardTitle>
                {available &&
                  ((stats.settling ?? 0) > 0 || (stats.reconcile ?? 0) > 0) && (
                    <p className="text-sm text-muted-foreground">
                      等待结算 {stats.settling ?? 0} 条 · 待核对{' '}
                      {stats.reconcile ?? 0} 条。结算完成后才会发布并计入统计。
                    </p>
                  )}
                <CardDescription>
                  最近 50 条，死信可由管理员重新入队。
                </CardDescription>
              </div>
              <select
                value={status}
                disabled={!runtimeMode?.outboxEnabled || loading}
                onChange={(event) => setStatus(event.target.value)}
                className="h-9 rounded-lg border border-input bg-background px-3 text-sm"
              >
                <option value="">全部状态</option>
                <option value="PENDING">待发布</option>
                <option value="SETTLING">等待结算</option>
                <option value="RECONCILE">待核对</option>
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
                        <Button
                          size="xs"
                          variant="ghost"
                          onClick={() =>
                            onInspectRequest({
                              requestId: record.event.requestId,
                            })
                          }
                        >
                          查看处理过程
                        </Button>
                        {record.status === 'DEAD_LETTERED' && (
                          <Button
                            size="xs"
                            variant="outline"
                            disabled={busy || !canAdmin || !available}
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

        <Card className="self-start bg-card text-card-foreground xl:sticky xl:top-24">
          <CardHeader>
            <CardTitle>Kafka 分区</CardTitle>
            <CardDescription>
              消费进度由运行中的 Consumer 实时上报。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {lag.length === 0 ? (
              <p className="rounded-lg border bg-card p-4 text-sm text-muted-foreground">
                暂无分区采样；Consumer 收到消息后会显示。
              </p>
            ) : (
              lag.map((item) => (
                <div
                  key={`${item.topic}-${item.partition}`}
                  className="rounded-lg border bg-card p-3"
                >
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-xs text-muted-foreground">
                      P{item.partition}
                    </span>
                    <span
                      className={
                        item.lag > 0 ? 'text-amber-700' : 'text-emerald-700'
                      }
                    >
                      {item.lag.toLocaleString()} lag
                    </span>
                  </div>
                  <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted">
                    <div
                      className={`h-full rounded-full ${item.lag > 0 ? 'bg-amber-300' : 'bg-emerald-300'}`}
                      style={{
                        width: `${item.lag > 0 ? Math.min(100, 12 + Math.log10(item.lag + 1) * 28) : 4}%`,
                      }}
                    />
                  </div>
                  <p className="mt-2 truncate text-xs text-muted-foreground">
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

function MetricCard({
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
function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
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
      className={`block space-y-1.5 text-sm font-medium ${dark ? 'text-foreground' : 'text-muted-foreground'} ${className}`}
    >
      <span className="block">{label}</span>
      {children}
    </label>
  );
}
function Empty({ text, action }: { text: string; action?: React.ReactNode }) {
  return (
    <div className="flex min-h-48 flex-col items-center justify-center gap-3 px-6 text-center text-sm text-muted-foreground">
      <Inbox className="size-5 text-muted-foreground/60" />
      <p>{text}</p>
      {action}
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
