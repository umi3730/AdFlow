'use client';

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  useMemo,
  type SetStateAction,
} from 'react';
import { Download, Play, RefreshCw, Square, UsersRound } from 'lucide-react';
import {
  api,
  newClientID,
  simulationAPIBase,
  type Campaign,
  type Profile,
  type TraceTarget,
} from '@/lib/api';
import {
  localSimulationTarget,
  makeSimulationProfiles,
  simulationLimits,
  startSimulation,
  validateSimulationConfig,
  type SimulationConfig,
  type SimulationSnapshot,
} from '@/lib/user-pool-simulator';
import { Button } from '@/components/ui/button';
import { SimulationResults } from '@/components/simulation-results';
import { PageHeading } from '@/components/page-heading';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { FormSelect } from '@/components/form-select';
import {
  refreshSimulationPool,
  type SimulationPool,
  type PoolSource,
} from '@/lib/simulation-pool';
import { profileDeviceLabel, profileTagLabel } from '@/lib/profile-options';
import {
  makePlanSimulationProfiles,
  type SampleMix,
} from '@/lib/plan-simulation-profiles';
import { campaignDisplayStatus } from '@/lib/campaign-delivery';
import { simulationMoneyFen } from '@/lib/simulation-behavior';
import { adSlotOptions, defaultAdSlotID } from '@/lib/ad-slots';
import { demoSlotID, loadDemoProfiles } from '@/lib/demo-data';
import { checkDemoReadiness } from '@/lib/demo-readiness';
import { simulationTraceTarget } from '@/lib/request-trace';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';

export function UserPoolSimulation({
  now,
  active,
  campaigns,
  onFinished,
  onRunningChange,
  stopSignal = 0,
  demoRequest = 0,
  onInspectRequest,
}: {
  now: number | null;
  active: boolean;
  demoRequest?: number;
  onInspectRequest?: (target: TraceTarget) => void;
  campaigns: Campaign[];
  onFinished: () => Promise<void>;
  onRunningChange?: (running: boolean) => void;
  stopSignal?: number;
}) {
  const [pools, setPools] = useState<Record<PoolSource, SimulationPool>>({
    temporary: { profiles: [], selectedIds: [] },
    saved: { profiles: [], selectedIds: [] },
  });
  const [seed, setSeed] = useState('adflow-1');
  const [sampleMode, setSampleMode] = useState('random');
  const [sampleCampaignId, setSampleCampaignId] = useState('');
  const [sampleMix, setSampleMix] = useState<SampleMix>('mixed');
  const [loading, setLoading] = useState(false);
  const [poolSource, setPoolSource] = useState<'temporary' | 'saved'>(
    'temporary',
  );
  const { profiles, selectedIds: userIds } = pools[poolSource];
  function setUserIds(update: SetStateAction<string[]>) {
    setPools((previous) => ({
      ...previous,
      [poolSource]: {
        ...previous[poolSource],
        selectedIds:
          typeof update === 'function'
            ? update(previous[poolSource].selectedIds)
            : update,
      },
    }));
  }
  const [poolSize, setPoolSize] = useState('20');
  const [slotId, setSlotId] = useState(defaultAdSlotID);
  const [maxRounds, setMaxRounds] = useState('3');
  const [concurrency, setConcurrency] = useState('1');
  const [seconds, setSeconds] = useState('30');
  const [timeoutMs, setTimeoutMs] = useState('5000');
  const [impressions, setImpressions] = useState(true);
  const [simulateBehavior, setSimulateBehavior] = useState(false);
  const [clickPercent, setClickPercent] = useState('50');
  const [conversionPercent, setConversionPercent] = useState('30');
  const [minValue, setMinValue] = useState('9.90');
  const [maxValue, setMaxValue] = useState('199.00');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [snapshot, setSnapshot] = useState<SimulationSnapshot | null>(null);
  const [runConfig, setRunConfig] = useState<SimulationConfig | null>(null);
  const [runContext, setRunContext] = useState<{
    poolSource: 'temporary' | 'saved';
    seed?: string;
    profiles: Profile[];
    campaigns: Campaign[];
    planSample?: SimulationPool['planSample'];
  } | null>(null);
  const [lastStartedAt, setLastStartedAt] = useState('');
  const runner = useRef<ReturnType<typeof startSimulation> | null>(null);
  const resultsAnchor = useRef<HTMLDivElement>(null);
  const preview = useMemo(
    () => ({
      seconds: Number(seconds),
      maxRounds: Number(maxRounds),
      concurrency: Number(concurrency),
    }),
    [seconds, maxRounds, concurrency],
  );
  const loadController = useRef<AbortController | null>(null);
  const skipSavedRefresh = useRef(false);
  const mounted = useRef(false);
  const local = localSimulationTarget(simulationAPIBase);
  const running =
    snapshot?.status === 'running' || snapshot?.status === 'stopping';
  const locked = running;
  useEffect(() => {
    onRunningChange?.(running);
  }, [running, onRunningChange]);

  const loadProfiles = useCallback(async () => {
    loadController.current?.abort();
    const controller = new AbortController();
    loadController.current = controller;
    setLoading(true);
    setError('');
    try {
      const page = await api.listProfiles({ limit: 100 }, controller.signal);
      if (!controller.signal.aborted && mounted.current) {
        setPools((previous) => ({
          ...previous,
          saved: refreshSimulationPool(previous.saved, page.items),
        }));
        setNotice(page.total > 100 ? '仅显示前 100 位已保存用户。' : '');
      }
    } catch (cause) {
      if (!controller.signal.aborted && mounted.current)
        setError(cause instanceof Error ? cause.message : '用户池读取失败');
    } finally {
      if (mounted.current && !controller.signal.aborted) setLoading(false);
    }
  }, []);

  const stop = useCallback((reason = 'manual') => {
    runner.current?.stop(reason);
  }, []);
  useEffect(() => {
    if (stopSignal > 0) stop('manual');
  }, [stopSignal, stop]);
  useEffect(() => {
    mounted.current = true;
    const hidden = () => {
      if (document.hidden) stop('hidden');
    };
    document.addEventListener('visibilitychange', hidden);
    return () => {
      mounted.current = false;
      document.removeEventListener('visibilitychange', hidden);
      stop('unmounted');
      loadController.current?.abort();
    };
  }, [stop]);
  useEffect(() => {
    let live = true;
    void Promise.resolve().then(() => {
      if (!live) return;
      if (active && poolSource === 'saved' && !runner.current) {
        if (skipSavedRefresh.current) {
          skipSavedRefresh.current = false;
          return;
        }
        void loadProfiles();
      }
    });
    return () => {
      live = false;
    };
  }, [active, poolSource, loadProfiles]);

  const loadDemoCase = useCallback(
    async (kind: 'rules' | 'auction' = 'rules') => {
      if (runner.current || loading) return;
      loadController.current?.abort();
      const controller = new AbortController();
      loadController.current = controller;
      setLoading(true);
      setError('');
      setNotice('');
      try {
        const existing =
          kind === 'auction'
            ? [await api.getProfile('demo-auction-user', controller.signal)]
            : await loadDemoProfiles((id) =>
                api.getProfile(id, controller.signal),
              );
        if (controller.signal.aborted || !mounted.current) return;
        if (!existing.length)
          throw new Error(
            '演示用户不存在或已删除。请使用自己的画像或生成临时用户。',
          );
        const readiness = await checkDemoReadiness(
          kind,
          api.getCampaign,
          api.listCreatives,
        );
        if (controller.signal.aborted || !mounted.current) return;
        skipSavedRefresh.current = poolSource !== 'saved';
        setPoolSource('saved');
        setPools((previous) => ({
          ...previous,
          saved: {
            profiles: existing,
            selectedIds: existing.map((profile) => profile.userId),
          },
        }));
        setSlotId(demoSlotID);
        setConcurrency(kind === 'auction' ? '1' : String(existing.length));
        setMaxRounds(kind === 'auction' ? '3' : String(existing.length));
        setSeconds('30');
        setImpressions(true);
        setSimulateBehavior(true);
        setClickPercent('100');
        setConversionPercent('100');
        setMinValue('5.00');
        setMaxValue('5.00');
        setNotice(
          kind === 'rules' && existing.length < 3
            ? '部分演示用户已删除，仅载入仍存在的用户。'
            : readiness,
        );
      } catch (cause) {
        if (!controller.signal.aborted && mounted.current)
          setError(cause instanceof Error ? cause.message : '演示用例读取失败');
      } finally {
        if (!controller.signal.aborted && mounted.current) setLoading(false);
      }
    },
    [loading, poolSource],
  );

  const handledDemoRequest = useRef(0);
  useEffect(() => {
    if (
      !active ||
      loading ||
      !demoRequest ||
      handledDemoRequest.current === demoRequest
    )
      return;
    handledDemoRequest.current = demoRequest;
    void loadDemoCase('auction');
  }, [active, loading, demoRequest, loadDemoCase]);

  async function generatePool(nextSeed = seed) {
    if (runner.current || loading || !local) return;
    loadController.current?.abort();
    const controller = new AbortController();
    loadController.current = controller;
    setLoading(true);
    setError('');
    try {
      let generated: Profile[];
      let planSample: SimulationPool['planSample'];
      if (sampleMode === 'plan') {
        if (!sampleCampaignId) throw new Error('请先选择用于生成样本的计划');
        const campaign = await api.getCampaign(sampleCampaignId);
        if (controller.signal.aborted || !mounted.current) return;
        const result = makePlanSimulationProfiles(
          Number(poolSize),
          nextSeed,
          campaign,
          sampleMix,
        );
        const creatives = await api.listCreatives(campaign.id);
        if (controller.signal.aborted || !mounted.current) return;
        generated = result.profiles;
        planSample = result.context;
        if (campaignDisplayStatus(campaign, Date.now()) !== 'ACTIVE')
          planSample.warnings.push(
            '该计划当前不在投放中，样本可生成，但运行时不会选中该计划。',
          );
        if (!creatives.items.some((creative) => creative.status === 'ACTIVE'))
          planSample.warnings.push(
            '该计划没有启用的素材，请在素材管理中添加并启用素材后再测试投放。',
          );
        setSlotId(campaign.slotId);
      } else {
        generated = makeSimulationProfiles(Number(poolSize), nextSeed);
      }
      setPoolSource('temporary');
      setSeed(nextSeed);
      setPools((previous) => ({
        ...previous,
        temporary: {
          profiles: generated,
          selectedIds: generated.map((profile) => profile.userId),
          seed: nextSeed,
          planSample,
        },
      }));
      setNotice('');
    } catch (cause) {
      if (!controller.signal.aborted && mounted.current)
        setError(cause instanceof Error ? cause.message : '生成失败');
    } finally {
      if (!controller.signal.aborted && mounted.current) setLoading(false);
    }
  }

  function start(event: { preventDefault(): void }) {
    event.preventDefault();
    if (runner.current || loading) return;
    setError('');
    setNotice('');
    try {
      if (document.hidden) throw new Error('请在前台页面启动模拟。');
      if (!local)
        throw new Error(
          '本页只允许对本机后端发起模拟，不允许对远程地址压流量。',
        );
      const config = validateSimulationConfig({
        mode: 'concurrency',
        maxRounds: Number(maxRounds),
        runId: newClientID('sim'),
        userIds,
        slotId,
        concurrency: Number(concurrency),
        seconds: Number(seconds),
        timeoutMs: Number(timeoutMs),
        impressions,
        behavior:
          simulateBehavior && impressions
            ? {
                clickPercent: Number(clickPercent),
                conversionPercent: Number(conversionPercent),
                minValueFen: simulationMoneyFen(minValue),
                maxValueFen: simulationMoneyFen(maxValue),
              }
            : undefined,
      });
      setRunConfig(config);
      setRunContext(
        structuredClone({
          poolSource,
          seed: poolSource === 'temporary' ? pools.temporary.seed : undefined,
          planSample:
            poolSource === 'temporary' ? pools.temporary.planSample : undefined,
          profiles: profiles.filter((profile) =>
            config.userIds.includes(profile.userId),
          ),
          campaigns: campaigns.filter(
            (campaign) => campaign.slotId === config.slotId,
          ),
        }),
      );
      setLastStartedAt(new Date().toISOString());
      const profileById = new Map(
        profiles.map((profile) => [profile.userId, profile]),
      );
      const handle = startSimulation(
        config,
        {
          decide: (input, signal) => {
            if (poolSource === 'saved') return api.decide(input, signal);
            const profile = profileById.get(input.userId);
            if (!profile) return Promise.reject(new Error('临时用户不存在'));
            return api.simulateDecision(
              {
                runId: config.runId,
                requestId: input.requestId,
                slotId: input.slotId,
                profile,
              },
              signal,
            );
          },
          impression: (decision, eventId, signal) =>
            api.recordEvent(
              {
                eventId,
                requestId: decision.requestId,
                campaignId: decision.campaignId,
                creativeId: decision.creativeId,
                type: 'impression',
              },
              signal,
            ),
          click: (decision, eventId, signal) =>
            api.recordEvent(
              {
                eventId,
                requestId: decision.requestId,
                campaignId: decision.campaignId,
                creativeId: decision.creativeId,
                type: 'click',
              },
              signal,
            ),
          conversion: (decision, eventId, valueFen, signal) =>
            api.recordEvent(
              {
                eventId,
                requestId: decision.requestId,
                campaignId: decision.campaignId,
                creativeId: decision.creativeId,
                type: 'conversion',
                valueFen,
              },
              signal,
            ),
        },
        (result) => {
          if (mounted.current) setSnapshot(result);
        },
      );
      runner.current = handle;
      requestAnimationFrame(() =>
        resultsAnchor.current?.scrollIntoView({
          block: 'start',
          behavior: 'auto',
        }),
      );
      void handle.done.then(async () => {
        if (runner.current === handle) runner.current = null;
        if (mounted.current) await onFinished();
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '启动失败');
    }
  }

  function exportReport() {
    if (!snapshot || !runConfig) return;
    const report = {
      kind: 'browser-business-simulation',
      schemaVersion: 2,
      exportedAt: new Date().toISOString(),
      startedAt: lastStartedAt,
      config: runConfig,
      context: runContext,
      result: snapshot,
      note: 'Browser-side timing includes network and selected impression/click/conversion events. Conversion values are synthetic, not real revenue. Accepted events may still await asynchronous processing. Not a server capacity benchmark. No-Ad is not an HTTP error. Stop does not roll back accepted requests.',
    };
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(report, null, 2)], { type: 'application/json' }),
    );
    const link = document.createElement('a');
    link.href = url;
    link.download = snapshot.runId + '.json';
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  return (
    <>
      <PageHeading
        title="批量投放测试"
        description="选择用户，以固定并发运行广告决策，查看命中与事件回传结果。"
        action={
          <Button
            type="button"
            variant="outline"
            onClick={exportReport}
            disabled={!snapshot || locked}
          >
            <Download />
            导出结果
          </Button>
        }
      />
      {running && (
        <output className="sticky top-28 z-10 mt-3 flex flex-wrap items-center justify-between gap-2 rounded-md border bg-card p-3 shadow-sm lg:top-14">
          <span className="text-sm">
            {snapshot?.status === 'stopping'
              ? '正在结束，等待在途轮次…'
              : '固定并发模拟运行中'}
          </span>
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={snapshot?.status !== 'running'}
              onClick={() => runner.current?.drain()}
            >
              结束并等待
            </Button>
            <Button size="sm" variant="outline" onClick={() => stop()}>
              立即停止
            </Button>
          </div>
        </output>
      )}
      {!local && (
        <p role="alert" className="mt-3 text-sm text-destructive">
          当前后端不是本机地址，已禁用模拟与批量生成。
        </p>
      )}
      {error && (
        <p
          role="alert"
          className="mt-3 rounded-lg bg-destructive/10 p-3 text-sm text-destructive"
        >
          {error}
        </p>
      )}
      <div className="mt-5 grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <UsersRound className="size-4" />
              1. {poolSource === 'temporary' ? '临时用户池' : '已保存用户'} ·
              已选 {userIds.length} 人
            </CardTitle>
            <CardDescription>
              {poolSource === 'temporary'
                ? '最多 100 人，仅本页内存；不测画像数据库/缓存读取。'
                : '使用已有画像，不新建或改写用户。'}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {notice && (
              <p className="text-xs text-muted-foreground">{notice}</p>
            )}
            <div className="space-y-2 border-b pb-3">
              <Button
                type="button"
                variant="outline"
                disabled={locked || loading}
                onClick={() => void loadDemoCase()}
              >
                载入演示用例
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={
                  locked ||
                  loading ||
                  !campaigns.some((c) => c.activeVersion?.auction)
                }
                onClick={() => void loadDemoCase('auction')}
              >
                载入竞价用例
              </Button>
              <p className="text-xs leading-5 text-muted-foreground">
                使用内置的命中、排除和未命中画像，各运行一次。演示数据可在计划、素材和画像页面修改或删除。
              </p>
            </div>
            <FormSelect
              label="用户池来源"
              value={poolSource}
              disabled={locked}
              options={[
                { value: 'temporary', label: '临时用户（不保存）' },
                { value: 'saved', label: '已保存用户' },
              ]}
              onChange={(value) => {
                if (runner.current) return;
                const next = value === 'saved' ? 'saved' : 'temporary';
                if (next === poolSource) return;
                loadController.current?.abort();
                setPoolSource(next);
                setLoading(false);
                setNotice('');
              }}
            />
            {poolSource === 'temporary' && (
              <>
                <FormSelect
                  label="样本生成方式"
                  value={sampleMode}
                  disabled={locked || loading}
                  options={[
                    { value: 'random', label: '随机样本' },
                    { value: 'plan', label: '按计划生成样本' },
                  ]}
                  onChange={setSampleMode}
                />
                {sampleMode === 'plan' ? (
                  <div className="space-y-3">
                    <FormSelect
                      label="参考计划"
                      value={sampleCampaignId}
                      disabled={locked || loading}
                      options={campaigns
                        .filter((campaign) => campaign.activeVersion)
                        .map((campaign) => ({
                          value: campaign.id,
                          label: campaign.name,
                        }))}
                      onChange={setSampleCampaignId}
                      placeholder="请选择已发布规则的计划"
                    />
                    {!campaigns.some((campaign) => campaign.activeVersion) && (
                      <p className="text-xs text-muted-foreground">
                        暂无已发布规则的计划，请先创建并发布计划。
                      </p>
                    )}
                    <FormSelect
                      label="样本组合"
                      value={sampleMix}
                      disabled={locked || loading}
                      options={[
                        {
                          value: 'mixed',
                          label: '满足与不满足各半（奇数多一个满足）',
                        },
                        { value: 'matched', label: '全部满足定向' },
                        { value: 'unmatched', label: '全部不满足定向' },
                      ]}
                      onChange={(value) => setSampleMix(value as SampleMix)}
                    />
                    <p className="text-xs leading-5 text-muted-foreground">
                      按所选计划的已发布规则生成，包含自定义标签；生成后自动切换到该计划的广告位。
                    </p>
                  </div>
                ) : (
                  <p className="text-xs leading-5 text-muted-foreground">
                    从常用兴趣、设备、年龄等范围随机取样，不根据计划补标签，也不保证满足定向。
                  </p>
                )}
                <label htmlFor="sim-seed" className="block space-y-1 text-sm">
                  样本种子
                  <Input
                    id="sim-seed"
                    aria-label="样本种子"
                    value={seed}
                    onChange={(event) => setSeed(event.target.value)}
                    maxLength={80}
                    disabled={locked || loading}
                  />
                </label>
                <div className="flex flex-wrap gap-2">
                  <Input
                    type="number"
                    aria-label="生成用户数"
                    className="min-w-0 flex-1 basis-24"
                    value={poolSize}
                    onChange={(event) => setPoolSize(event.target.value)}
                    min={1}
                    max={100}
                    disabled={locked || loading}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    disabled={
                      locked ||
                      loading ||
                      !local ||
                      (sampleMode === 'plan' && !sampleCampaignId)
                    }
                    onClick={() => void generatePool()}
                  >
                    {loading ? '正在生成…' : '生成临时用户'}
                  </Button>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={
                    locked ||
                    loading ||
                    !local ||
                    (sampleMode === 'plan' && !sampleCampaignId)
                  }
                  onClick={() => void generatePool(newClientID('pool'))}
                >
                  换一批样本
                </Button>
                <p className="text-xs text-muted-foreground">
                  相同规则、样本组合、人数与种子可复现画像；修改选项后需重新生成，不写入画像表。
                </p>
                {pools.temporary.planSample && profiles.length > 0 && (
                  <div
                    className="space-y-2 border-t pt-3 text-xs leading-5"
                    aria-live="polite"
                  >
                    <p className="break-words">
                      当前样本：{pools.temporary.planSample.campaignName} · v
                      {pools.temporary.planSample.version}。 满足定向{' '}
                      {
                        profiles.filter(
                          (profile) =>
                            pools.temporary.planSample!.expected[
                              profile.userId
                            ],
                        ).length
                      }{' '}
                      人， 不满足{' '}
                      {
                        profiles.filter(
                          (profile) =>
                            !pools.temporary.planSample!.expected[
                              profile.userId
                            ],
                        ).length
                      }{' '}
                      人。
                    </p>
                    <p className="text-muted-foreground">
                      仅针对生成时的所选计划；满足定向仍受素材、预算、频控和竞价影响。不满足的用户可能命中其他计划。
                    </p>
                    {slotId !== pools.temporary.planSample.slotId && (
                      <p className="text-amber-800">
                        当前广告位与参考计划不同，请切回原广告位或重新生成样本。
                      </p>
                    )}
                    {pools.temporary.planSample.warnings.map((warning) => (
                      <p key={warning} className="text-amber-800">
                        {warning}
                      </p>
                    ))}
                  </div>
                )}
              </>
            )}
            <div className="flex flex-wrap gap-2">
              {poolSource === 'saved' && (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={locked || loading}
                  onClick={() => {
                    setPoolSource('saved');
                    if (poolSource === 'saved') void loadProfiles();
                  }}
                >
                  <RefreshCw />
                  刷新列表
                </Button>
              )}

              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={locked || loading}
                onClick={() =>
                  setUserIds(profiles.map((profile) => profile.userId))
                }
              >
                全选
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={locked}
                onClick={() => setUserIds([])}
              >
                清空选择
              </Button>
              {poolSource === 'temporary' && (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  disabled={locked || loading}
                  onClick={() => {
                    setPools((previous) => ({
                      ...previous,
                      temporary: { profiles: [], selectedIds: [] },
                    }));
                    setNotice('');
                  }}
                >
                  清空临时池
                </Button>
              )}
            </div>
            {loading ? (
              <p className="text-sm text-muted-foreground">正在准备用户池…</p>
            ) : !profiles.length ? (
              <p className="text-sm text-muted-foreground">
                {poolSource === 'temporary'
                  ? '先生成临时用户，生成后自动全选。'
                  : '没有已保存用户。可以在用户画像页创建，或切换到临时用户。'}
              </p>
            ) : (
              <details className="border-t pt-3">
                <summary className="cursor-pointer text-sm font-medium">
                  查看用户与调整选择（{profiles.length} 人）
                </summary>
                <div className="mt-3 max-h-64 space-y-2 overflow-y-auto p-1">
                  {profiles.map((profile) => (
                    <label
                      key={profile.userId}
                      htmlFor={'sim-pool-' + encodeURIComponent(profile.userId)}
                      className="flex items-start gap-2 rounded-lg border p-2"
                    >
                      <Checkbox
                        id={'sim-pool-' + encodeURIComponent(profile.userId)}
                        className="mt-1"
                        disabled={locked}
                        checked={userIds.includes(profile.userId)}
                        onCheckedChange={(checked) =>
                          setUserIds((previous) =>
                            checked
                              ? [...new Set([...previous, profile.userId])]
                              : previous.filter((id) => id !== profile.userId),
                          )
                        }
                      />
                      <span className="min-w-0">
                        <span className="block break-all text-sm">
                          {profile.userId}
                          {poolSource === 'temporary' &&
                            pools.temporary.planSample && (
                              <span className="ml-2 text-xs text-muted-foreground">
                                {pools.temporary.planSample.expected[
                                  profile.userId
                                ]
                                  ? '满足参考计划定向'
                                  : '不满足参考计划定向'}
                              </span>
                            )}
                        </span>
                        <span className="block text-xs text-muted-foreground">
                          {profileDeviceLabel(
                            profile.fields.device || '无设备',
                          )}{' '}
                          · 年龄 {profile.fields.age || '—'} · 分数{' '}
                          {profile.fields.score || '—'}
                        </span>
                        <span className="block break-words text-xs text-muted-foreground">
                          标签：
                          {profile.tags.length
                            ? profile.tags.map(profileTagLabel).join('、')
                            : '无'}
                        </span>
                      </span>
                    </label>
                  ))}
                </div>
              </details>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>2. 运行配置</CardTitle>
            <CardDescription>每轮一次决策，按用户池顺序轮询。</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="mb-4 space-y-2 border-b pb-4 text-sm">
              <p>
                已选 <strong>{userIds.length}</strong> 人 · 当前广告位{' '}
                <strong>
                  {
                    campaigns.filter(
                      (c) =>
                        c.slotId === slotId &&
                        campaignDisplayStatus(c, now) === 'ACTIVE',
                    ).length
                  }
                </strong>{' '}
                个投放中计划
              </p>
              {campaigns.filter(
                (c) =>
                  c.slotId === slotId &&
                  campaignDisplayStatus(c, now) === 'ACTIVE',
              ).length === 0 && (
                <p className="text-amber-800">
                  该广告位暂无投放中计划，运行可能全部未命中；仍可用于验证无广告场景。
                </p>
              )}
              <p className="text-xs leading-5 text-muted-foreground">
                {poolSource === 'temporary'
                  ? '临时用户每次运行使用新身份，同次运行内沿用频控；计划预算累计。'
                  : '已保存用户沿用已有身份，频控和计划预算均累计。'}
              </p>
            </div>
            <form onSubmit={start} className="space-y-4">
              <fieldset disabled={locked} className="space-y-3">
                <label htmlFor="sim-slot" className="block space-y-1 text-sm">
                  广告位
                  <FormSelect
                    id="sim-slot"
                    label="广告位"
                    value={slotId}
                    options={adSlotOptions}
                    onChange={setSlotId}
                    disabled={locked}
                  />
                </label>
                <div className="grid grid-cols-2 gap-3">
                  <label htmlFor="sim-rounds" className="space-y-1 text-sm">
                    最大轮数
                    <Input
                      id="sim-rounds"
                      type="number"
                      value={maxRounds}
                      onChange={(event) => setMaxRounds(event.target.value)}
                      min={1}
                      max={simulationLimits.rounds}
                      required
                    />
                  </label>
                  <label
                    htmlFor="sim-concurrency"
                    className="space-y-1 text-sm"
                  >
                    目标并发轮次
                    <Input
                      id="sim-concurrency"
                      type="number"
                      value={concurrency}
                      onChange={(event) => setConcurrency(event.target.value)}
                      min={1}
                      max={simulationLimits.concurrency}
                      required
                    />
                  </label>
                  <label htmlFor="sim-seconds" className="space-y-1 text-sm">
                    最长派发时间（秒）
                    <Input
                      id="sim-seconds"
                      type="number"
                      value={seconds}
                      onChange={(event) => setSeconds(event.target.value)}
                      min={1}
                      max={simulationLimits.seconds}
                      required
                    />
                  </label>
                  <label htmlFor="sim-timeout" className="space-y-1 text-sm">
                    单轮超时（毫秒）
                    <Input
                      id="sim-timeout"
                      type="number"
                      value={timeoutMs}
                      onChange={(event) => setTimeoutMs(event.target.value)}
                      min={100}
                      max={10000}
                      required
                    />
                  </label>
                </div>
                <label
                  htmlFor="sim-impressions"
                  className="flex items-center gap-2 text-sm"
                >
                  <Checkbox
                    id="sim-impressions"
                    checked={impressions}
                    onCheckedChange={(checked) =>
                      setImpressions(Boolean(checked))
                    }
                    disabled={locked}
                  />
                  命中后自动回传曝光
                </label>
                {impressions && (
                  <>
                    <label
                      htmlFor="sim-behavior"
                      className="flex items-center gap-2 text-sm"
                    >
                      <Checkbox
                        id="sim-behavior"
                        checked={simulateBehavior}
                        disabled={locked}
                        onCheckedChange={(checked) =>
                          setSimulateBehavior(Boolean(checked))
                        }
                      />
                      模拟点击与转化
                    </label>
                    {simulateBehavior && (
                      <div className="space-y-3 rounded-lg border bg-muted/40 p-3">
                        <div className="grid grid-cols-2 gap-3">
                          <label
                            htmlFor="sim-click-percent"
                            className="space-y-1 text-sm"
                          >
                            曝光后点击率（%）
                            <Input
                              id="sim-click-percent"
                              type="number"
                              value={clickPercent}
                              onChange={(e) => setClickPercent(e.target.value)}
                              min={0}
                              max={100}
                              step={1}
                              required
                            />
                          </label>
                          <label
                            htmlFor="sim-conversion-percent"
                            className="space-y-1 text-sm"
                          >
                            点击后转化率（%）
                            <Input
                              id="sim-conversion-percent"
                              type="number"
                              value={conversionPercent}
                              onChange={(e) =>
                                setConversionPercent(e.target.value)
                              }
                              min={0}
                              max={100}
                              step={1}
                              required
                            />
                          </label>
                          <label
                            htmlFor="sim-min-value"
                            className="space-y-1 text-sm"
                          >
                            金额下限（元）
                            <Input
                              id="sim-min-value"
                              type="number"
                              value={minValue}
                              onChange={(e) => setMinValue(e.target.value)}
                              min={0.01}
                              max={10000}
                              step="0.01"
                              required
                            />
                          </label>
                          <label
                            htmlFor="sim-max-value"
                            className="space-y-1 text-sm"
                          >
                            金额上限（元）
                            <Input
                              id="sim-max-value"
                              type="number"
                              value={maxValue}
                              onChange={(e) => setMaxValue(e.target.value)}
                              min={0.01}
                              max={10000}
                              step="0.01"
                              required
                            />
                          </label>
                        </div>
                        <p className="text-xs text-muted-foreground">
                          每次转化在范围内随机取值，计入测试计划的转化价值；不是预算充值或真实收入。
                        </p>
                      </div>
                    )}
                  </>
                )}
              </fieldset>
              <p className="text-xs text-muted-foreground">
                每轮包含决策和所选事件回传；达到轮数或时长上限后停止派发。
              </p>
              {!userIds.length && (
                <p className="text-sm text-muted-foreground">
                  先在用户池中生成或勾选至少一位用户。
                </p>
              )}
              <div className="flex flex-wrap gap-2">
                <Button
                  type="submit"
                  disabled={locked || loading || !local || !userIds.length}
                >
                  <Play />
                  {snapshot ? '再次运行' : '开始模拟'}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={snapshot?.status !== 'running'}
                  onClick={() => runner.current?.drain()}
                >
                  结束并等待
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={!locked}
                  onClick={() => stop()}
                >
                  <Square />
                  立即停止
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
        <div ref={resultsAnchor} className="min-w-0 scroll-mt-40 xl:col-span-2">
          <SimulationResults
            snapshot={snapshot}
            runConfig={runConfig}
            preview={preview}
            startedAt={lastStartedAt}
            onInspectRequest={
              onInspectRequest
                ? (requestId, userId, outcome) =>
                    onInspectRequest({
                      ...simulationTraceTarget(
                        requestId,
                        userId,
                        runConfig?.runId ?? '',
                        runContext?.poolSource === 'temporary',
                      ),
                      clientOutcome: outcome,
                    })
                : undefined
            }
          />
        </div>
      </div>
    </>
  );
}
