'use client';

import { useEffect, useRef, useState } from 'react';
import { LoaderCircle, Plus, RotateCcw } from 'lucide-react';
import { api, Campaign, Metrics } from '@/lib/api';
import { campaignDisplayStatus } from '@/lib/campaign-delivery';
import { CampaignRuleDialog } from '@/components/campaign-rule-dialog';
import { FormSelect } from '@/components/form-select';
import { useAccess } from '@/components/auth-gate';
import {
  allCampaignFilters,
  campaignStatusOptions,
  filterCampaigns,
} from '@/lib/campaign-filters';
import { adSlotOptions, defaultAdSlotID } from '@/lib/ad-slots';
import { DeleteResourceButton } from '@/components/delete-resource-button';
import { newAgentCampaignInput } from '@/lib/agent-campaign';
import type { RuleEditorValue } from '@/lib/campaign-rules';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { PageHeading } from '@/components/page-heading';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog';
import { CampaignTable, Field } from './shared';
import {
  useConsoleLocation,
  updateConsoleLocation,
} from '@/hooks/use-console-location';

export function CampaignsView({
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
  onAddMaterials,
  onTest,
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
  onAddMaterials: (campaign: Campaign) => void;
  onTest: (campaign: Campaign) => void;
}) {
  const { canOperate, canAdmin } = useAccess();
  const [nameOverride, setName] = useState<string | null>(null);
  const [createError, setCreateError] = useState('');
  const createTriggerRef = useRef<HTMLButtonElement>(null);
  const { slot: filterSlot, status: filterStatus } = useConsoleLocation();
  const setFilterSlot = (slot: string) => updateConsoleLocation({ slot });
  const setFilterStatus = (status: string) => updateConsoleLocation({ status });
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
    updateConsoleLocation({ q: '', slot: '', status: '' });
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
        setSelectedCampaign(created);
      }, '草稿已创建，请配置投放规则');
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
            <div className="flex flex-wrap justify-end gap-1">
              {campaign.status === 'ACTIVE' && canOperate && (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy || campaign.activeCreativeCount == null}
                  onClick={() =>
                    campaign.activeCreativeCount === 0
                      ? onAddMaterials(campaign)
                      : onTest(campaign)
                  }
                >
                  {campaign.activeCreativeCount === 0 ? '添加素材' : '试投'}
                </Button>
              )}
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
          activeCreativeCount={
            campaigns.find((item) => item.id === selectedCampaign.id)
              ?.activeCreativeCount
          }
          onAddMaterials={() => onAddMaterials(selectedCampaign)}
          onTest={() => onTest(selectedCampaign)}
        />
      )}
    </>
  );
}
