'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Boxes,
  CheckCircle2,
  DatabaseZap,
  CircleHelp,
  FlaskConical,
  Layers3,
  LoaderCircle,
  RadioTower,
  RefreshCw,
  Search,
  Sparkles,
  UsersRound,
  X,
} from 'lucide-react';
import { Campaign, simulationAPIBase, type TraceTarget } from '@/lib/api';
import { useCampaignClock } from '@/hooks/use-campaign-clock';
import { useAdFlowTools } from '@/hooks/use-adflow-tools';

import { RequestTraceDialog } from '@/components/request-trace-dialog';
import { ConsoleHelp, type ConsoleView } from '@/components/console-guide';

import {
  beijingDate,
  reportDateRange,
  type DeliveryFilter,
} from '@/lib/delivery-report';
import { useAccess, SessionIdentity, RoleGate } from '@/components/auth-gate';
import { allCampaignFilters, filterCampaigns } from '@/lib/campaign-filters';
import { nextTestPlanNumber, testPlanName } from '@/lib/agent-campaign';
import type { RuleEditorValue } from '@/lib/campaign-rules';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { lazy, Suspense } from 'react';
import { Dashboard } from './console/dashboard';
import { useConsoleData } from '@/hooks/use-console-data';
import { defaultAdSlotID } from '@/lib/ad-slots';
import {
  useConsoleLocation,
  updateConsoleLocation,
} from '@/hooks/use-console-location';
import { reportLocation } from '@/lib/console-location';
const CampaignsView = lazy(() =>
  import('./console/campaigns-view').then((module) => ({
    default: module.CampaignsView,
  })),
);
const CreativesView = lazy(() =>
  import('./console/creatives-view').then((module) => ({
    default: module.CreativesView,
  })),
);
const DecisionView = lazy(() =>
  import('./console/decision-view').then((module) => ({
    default: module.DecisionView,
  })),
);
const OperationsView = lazy(() =>
  import('./console/operations-view').then((module) => ({
    default: module.OperationsView,
  })),
);
const ProfileWorkspace = lazy(() =>
  import('./profile-workspace').then((module) => ({
    default: module.ProfileWorkspace,
  })),
);
const UserPoolSimulation = lazy(() =>
  import('./user-pool-simulation').then((module) => ({
    default: module.UserPoolSimulation,
  })),
);
const AgentCenter = lazy(() =>
  import('./delivery-diagnosis').then((module) => ({
    default: module.AgentCenter,
  })),
);
const DeliveryReportWorkspace = lazy(() =>
  import('./delivery-report').then((module) => ({
    default: module.DeliveryReportWorkspace,
  })),
);

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
  { id: 'reports' as const, label: '效果报表', icon: Activity },
  { id: 'campaigns' as const, label: '广告计划', icon: Layers3 },
  { id: 'creatives' as const, label: '素材管理', icon: Boxes },
  { id: 'profiles' as const, label: '用户画像', icon: UsersRound },
  { id: 'decision' as const, label: '单次投放测试', icon: RadioTower },
  { id: 'simulation' as const, label: '批量投放测试', icon: FlaskConical },
  { id: 'operations' as const, label: '事件处理', icon: DatabaseZap },
  { id: 'agent' as const, label: 'Agent 助手', icon: Sparkles },
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
  const location = useConsoleLocation();
  const view = visibleNav.some((item) => item.id === location.view)
    ? location.view
    : 'dashboard';
  const [visitedViews, setVisitedViews] = useState<Set<View>>(
    () => new Set(['dashboard']),
  );
  if (!visitedViews.has(view))
    setVisitedViews(new Set([...visitedViews, view]));
  const [creativeCampaignID, setCreativeCampaignID] = useState('');
  const [userSelectionRevision, setUserSelectionRevision] = useState(0);
  const [decisionTarget, setDecisionTarget] = useState<Campaign | null>(null);
  const statsRefreshTimers = useRef<ReturnType<typeof setTimeout>[]>([]);
  function addMaterials(campaign: Campaign) {
    setCreativeCampaignID(campaign.id);
    setView('creatives');
  }
  function testCampaign(campaign: Campaign) {
    setDecisionTarget({ ...campaign });
    setDecisionSlotID(campaign.slotId);
    setUserSelectionRevision((revision) => revision + 1);
    setView('decision');
  }
  const [diagnosisFilter, setDiagnosisFilter] = useState<
    DeliveryFilter | undefined
  >();
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
  const [decisionUserID, setDecisionUserID] = useState('');
  const [decisionSlotID, setDecisionSlotID] = useState(defaultAdSlotID);
  const [pendingAgentDraft, setPendingAgentDraft] =
    useState<RuleEditorValue | null>(null);
  const [campaignDrafts, setCampaignDrafts] = useState<
    Record<string, RuleEditorValue>
  >({});
  const {
    campaigns,
    setCampaigns,
    metrics,
    connected,
    hasCheckedConnection,
    refreshing,
    lastUpdatedAt,
    refresh,
  } = useConsoleData();
  const now = useCampaignClock(campaigns);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [noticeTone, setNoticeTone] = useState<'success' | 'error'>('success');
  const setView = useCallback(
    (next: View) => {
      setNotice('');
      updateConsoleLocation({ view: next }, true);
      setVisitedViews((previous) =>
        previous.has(next) ? previous : new Set([...previous, next]),
      );
    },
    [setNotice, setVisitedViews],
  );
  const query = location.query;
  const setQuery = useCallback((q: string) => updateConsoleLocation({ q }), []);
  useEffect(() => {
    const clearNotice = () => setNotice('');
    window.addEventListener('popstate', clearNotice);
    return () => window.removeEventListener('popstate', clearNotice);
  }, []);

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

  useEffect(() => {
    if (!notice || noticeTone !== 'success') return;
    const timer = setTimeout(() => setNotice(''), 4500);
    return () => clearTimeout(timer);
  }, [notice, noticeTone]);
  useEffect(() => {
    if (view === 'dashboard' || view === 'campaigns') void refresh();
  }, [view, refresh]);
  useEffect(() => () => statsRefreshTimers.current.forEach(clearTimeout), []);
  const refreshAfterEvents = useCallback(async () => {
    statsRefreshTimers.current.forEach(clearTimeout);
    statsRefreshTimers.current = [1000, 3000, 8000].map((delay) =>
      setTimeout(() => {
        void refresh();
      }, delay),
    );
    await refresh();
  }, [refresh]);

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
            {connected ? '服务已连接' : '等待服务连接'}
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
            <Workspace>
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
                onAddMaterials={addMaterials}
                onTest={testCampaign}
                defaultName={testPlanName(testSequence)}
                agentDraft={pendingAgentDraft}
                campaignDrafts={campaignDrafts}
                onCreated={campaignCreated}
                onDiscardAgentDraft={() => setPendingAgentDraft(null)}
                onPublished={(campaign) => {
                  setNotice('');
                  setCampaignDrafts((previous) => {
                    const next = { ...previous };
                    delete next[campaign.id];
                    return next;
                  });
                }}
              />
            </Workspace>
          )}
          {view === 'reports' && (
            <Workspace>
              <DeliveryReportWorkspace
                campaigns={campaigns}
                refreshKey={lastUpdatedAt?.getTime() ?? 0}
                key={JSON.stringify(location.reportFilter)}
                initialFilter={location.reportFilter}
                onFilterApplied={(filter) =>
                  updateConsoleLocation(reportLocation(filter), true)
                }
                onDiagnose={(filter) => {
                  setDiagnosisFilter(filter);
                  setView('agent');
                }}
              />
            </Workspace>
          )}
          {(view === 'creatives' || visitedViews.has('creatives')) && (
            <div hidden={view !== 'creatives'}>
              <Workspace>
                <CreativesView
                  active={view === 'creatives'}
                  campaigns={campaigns}
                  busy={busy}
                  run={run}
                  campaignID={creativeCampaignID}
                  onCampaignIDChange={setCreativeCampaignID}
                  onTest={testCampaign}
                />
              </Workspace>
            </div>
          )}
          {view === 'profiles' && (
            <Workspace>
              <ProfileWorkspace
                onDecide={(userID) => {
                  setDecisionUserID(userID);
                  setUserSelectionRevision((revision) => revision + 1);
                  setView('decision');
                }}
              />
            </Workspace>
          )}
          {(view === 'decision' || visitedViews.has('decision')) && (
            <div hidden={view !== 'decision'}>
              <RoleGate minimum="operator">
                <Workspace>
                  <DecisionView
                    active={view === 'decision'}
                    onInspectRequest={inspectRequest}
                    busy={busy}
                    run={run}
                    key={userSelectionRevision}
                    userID={decisionUserID}
                    slotID={decisionSlotID}
                    onUserIDChange={setDecisionUserID}
                    onSlotIDChange={setDecisionSlotID}
                    campaigns={campaigns}
                    targetCampaign={decisionTarget}
                    onAddMaterials={addMaterials}
                    onEventsAccepted={refreshAfterEvents}
                    onReport={(campaignId) => {
                      const filter = reportDateRange(
                        beijingDate(),
                        beijingDate(),
                        'hour',
                      );
                      setDiagnosisFilter({ ...filter, campaignId });
                      updateConsoleLocation(
                        {
                          view: 'reports',
                          ...reportLocation({ ...filter, campaignId }),
                        },
                        true,
                      );
                    }}
                  />
                </Workspace>
              </RoleGate>
            </div>
          )}
          {view === 'operations' && (
            <Workspace>
              <OperationsView
                busy={busy}
                run={run}
                connected={connected}
                onInspectRequest={inspectRequest}
              />
            </Workspace>
          )}
          {(view === 'simulation' || visitedViews.has('simulation')) && (
            <div hidden={view !== 'simulation'}>
              <RoleGate minimum="admin">
                <Workspace>
                  <UserPoolSimulation
                    onInspectRequest={inspectRequest}
                    demoRequest={demoRequest}
                    now={now}
                    active={view === 'simulation'}
                    campaigns={campaigns}
                    onFinished={refreshAfterEvents}
                    onRunningChange={handleSimulationRunningChange}
                    stopSignal={simulationStopSignal}
                  />
                </Workspace>
              </RoleGate>
            </div>
          )}
          {view === 'agent' && (
            <RoleGate minimum="operator">
              <Workspace>
                <AgentCenter
                  campaigns={campaigns}
                  initialFilter={diagnosisFilter}
                  onViewReport={(filter) => {
                    setDiagnosisFilter(filter);
                    updateConsoleLocation(
                      { view: 'reports', ...reportLocation(filter) },
                      true,
                    );
                  }}
                  onCreatePlan={(value) => {
                    setPendingAgentDraft(structuredClone(value));
                    setQuery('');
                    setNotice('');
                    setNewPlanOpen(true);
                    setView('campaigns');
                  }}
                />
              </Workspace>
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

function Workspace({ children }: { children: React.ReactNode }) {
  return (
    <Suspense
      fallback={
        <output className="flex min-h-40 items-center justify-center gap-2 text-muted-foreground">
          <LoaderCircle className="size-5 animate-spin" />
          正在加载工作区…
        </output>
      }
    >
      {children}
    </Suspense>
  );
}
