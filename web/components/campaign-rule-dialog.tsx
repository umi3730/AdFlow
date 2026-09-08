'use client';

import { useRef, useState } from 'react';
import {
  AlertTriangle,
  CheckCircle2,
  LoaderCircle,
  Plus,
  Trash2,
} from 'lucide-react';
import { api, type Campaign, type Condition } from '@/lib/api';
import {
  campaignDisplayStatus,
  campaignDisplayLabels,
  formatCampaignDate,
} from '@/lib/campaign-delivery';
import { adSlotLabel } from '@/lib/ad-slots';
import { useAccess } from '@/components/auth-gate';
import {
  compileRuleDraft,
  editorFromCampaign,
  ruleGroups,
  type RuleEditorValue,
  type RuleGroup,
} from '@/lib/campaign-rules';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { TargetingSummary } from '@/components/targeting-summary';
import { RuleConditionInput } from '@/components/rule-condition-input';
import { FormSelect } from '@/components/form-select';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

const groups: Record<RuleGroup, { title: string; help: string }> = {
  all: { title: '必须全部满足', help: '用户需要同时满足这里的每一条条件。' },
  any: { title: '至少满足一条', help: '留空不限制；填写后至少命中其中一条。' },
  none: { title: '排除人群', help: '命中其中任何一条，就不投放。' },
};

export function CampaignRuleDialog({
  now,
  campaign,
  onClose,
  onChanged,
  initialValue,
  onPublished,
}: {
  now: number | null;
  campaign: Campaign;
  onClose: () => void;
  onChanged: () => Promise<void>;
  initialValue?: RuleEditorValue;
  onPublished?: (campaign: Campaign) => void;
}) {
  const { canAdmin, canOperate } = useAccess();
  const [current, setCurrent] = useState(campaign);
  const [value, setValue] = useState<RuleEditorValue>(() =>
    initialValue ? structuredClone(initialValue) : editorFromCampaign(campaign),
  );
  const [preview, setPreview] = useState<ReturnType<
    typeof compileRuleDraft
  > | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const contentRef = useRef<HTMLDivElement>(null);
  const editable =
    canAdmin && (current.status === 'DRAFT' || current.status === 'PAUSED');
  const version = current.activeVersion?.number ?? 0;
  const hasLegacyPlatform = ruleGroups.some((group) =>
    value.targeting[group].some((row) => row.field === 'platform'),
  );

  function change(next: RuleEditorValue) {
    setValue(next);
    setPreview(null);
    setError('');
    setSuccess('');
  }
  function changeRows(group: RuleGroup, rows: Condition[]) {
    change({ ...value, targeting: { ...value.targeting, [group]: rows } });
  }
  function updateRow(group: RuleGroup, index: number, row: Condition) {
    changeRows(
      group,
      value.targeting[group].map((existing, i) =>
        i === index ? row : existing,
      ),
    );
  }
  function review(event: { preventDefault(): void }) {
    event.preventDefault();
    try {
      setPreview(compileRuleDraft(value));
      setError('');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '规则格式错误');
      contentRef.current?.scrollTo({ top: 0 });
    }
  }
  async function pause() {
    setBusy(true);
    setError('');
    try {
      const paused = await api.pauseCampaign(current.id);
      setCurrent(paused);
      setValue(editorFromCampaign(paused));
      setSuccess('计划已暂停，现在可以编辑并发布下一版本。');
      await onChanged();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '暂停失败');
    } finally {
      setBusy(false);
      contentRef.current?.scrollTo({ top: 0 });
    }
  }
  async function publish() {
    if (!preview || !editable) return;
    setBusy(true);
    setError('');
    try {
      const published = await api.publishCampaign(current.id, preview);
      onPublished?.(published);
      setCurrent(published);
      setValue(editorFromCampaign(published));
      setPreview(null);
      setSuccess(
        `版本 v${published.activeVersion?.number ?? version + 1} 已发布；候选快照按缓存 TTL 刷新，默认约 5 秒。`,
      );
      await onChanged();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : '发布失败，修改内容已保留',
      );
    } finally {
      setBusy(false);
      contentRef.current?.scrollTo({ top: 0 });
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        ref={contentRef}
        className="max-h-[90dvh] overflow-y-auto p-5 sm:max-w-3xl sm:p-6"
        showCloseButton={!busy}
      >
        <DialogHeader>
          <div className="flex flex-wrap items-center gap-2 pr-8">
            <DialogTitle className="text-xl">
              {editable ? '编辑投放规则' : '查看投放规则'}
            </DialogTitle>
            <Badge variant="secondary">
              {version ? `当前 v${version}` : '尚未发布'}
            </Badge>
          </div>
          <DialogDescription>
            {current.name} · {adSlotLabel(current.slotId)}
          </DialogDescription>
        </DialogHeader>
        <div className="border-b pb-3 text-sm leading-6 text-muted-foreground">
          <p>
            {campaignDisplayLabels[campaignDisplayStatus(current, now)]} ·
            投放时间（北京时间）
          </p>
          <p>
            {formatCampaignDate(current.startAt)} 至{' '}
            {formatCampaignDate(current.endAt)}
          </p>
          {campaignDisplayStatus({ ...current, status: 'ACTIVE' }, now) ===
            'ENDED' && (
            <p className="text-amber-800">
              投放期已结束，重新发布规则不会延长期限。
            </p>
          )}
        </div>
        {success && (
          <output className="flex gap-2 rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800">
            <CheckCircle2 className="mt-0.5 size-4 shrink-0" />
            {success}
          </output>
        )}
        {error && (
          <p
            role="alert"
            className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800"
          >
            {error}
          </p>
        )}
        {!editable && (
          <div className="flex flex-col gap-3 rounded-lg border bg-muted/40 p-3 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-muted-foreground">
              当前版本只读。
              {!canAdmin
                ? '发布规则需要管理员权限。'
                : current.status === 'ACTIVE'
                  ? '暂停后可编辑，发布时生成新版本。'
                  : '当前状态不能发布。'}
            </p>
            {current.status === 'ACTIVE' && canOperate && (
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => void pause()}
              >
                {busy && <LoaderCircle className="animate-spin" />}暂停后编辑
              </Button>
            )}
          </div>
        )}
        {hasLegacyPlatform && (
          <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
            <p className="flex items-start gap-2">
              <AlertTriangle className="mt-0.5 size-4 shrink-0" />
              当前规则使用 platform，但画像编辑器保存的是
              device。字段名不同会导致定向不匹配。
            </p>
            {editable && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={busy}
                className="mt-2"
                onClick={() => {
                  const targeting = { ...value.targeting };
                  for (const group of ruleGroups)
                    targeting[group] = targeting[group].map((row) =>
                      row.field === 'platform'
                        ? { ...row, field: 'device' }
                        : row,
                    );
                  change({ ...value, targeting });
                }}
              >
                将 platform 条件改为 device
              </Button>
            )}
          </div>
        )}
        <form onSubmit={review} className="space-y-5">
          <fieldset
            disabled={!editable || busy || preview !== null}
            className="space-y-5 disabled:opacity-85"
          >
            <label htmlFor="rule-pricing" className="block space-y-1.5 text-sm">
              投放方式
              <FormSelect
                id="rule-pricing"
                label="投放方式"
                value={value.auction ? 'first_price' : 'fixed'}
                disabled={
                  !editable ||
                  busy ||
                  preview !== null ||
                  !current.auctionSupported
                }
                options={[
                  { value: 'fixed', label: '固定成本（兜底）' },
                  { value: 'first_price', label: '一价竞价（单次曝光）' },
                ]}
                onChange={(mode) =>
                  change({
                    ...value,
                    auction:
                      mode === 'first_price'
                        ? {
                            advertiserId: '',
                            advertiserName: '',
                            bidYuan: '0.05',
                          }
                        : undefined,
                  })
                }
              />
            </label>
            {!current.auctionSupported && (
              <p className="text-xs text-muted-foreground">
                当前后端尚未支持竞价，更新后可配置。
              </p>
            )}
            {value.auction && (
              <div className="grid gap-3 sm:grid-cols-2">
                <label
                  htmlFor="rule-advertiser-id"
                  className="space-y-1.5 text-sm"
                >
                  广告主标识
                  <Input
                    id="rule-advertiser-id"
                    value={value.auction.advertiserId}
                    maxLength={64}
                    placeholder="例如 studio-a"
                    required
                    onChange={(event) =>
                      change({
                        ...value,
                        auction: {
                          ...value.auction!,
                          advertiserId: event.target.value,
                        },
                      })
                    }
                  />
                </label>
                <label
                  htmlFor="rule-advertiser-name"
                  className="space-y-1.5 text-sm"
                >
                  广告主名称
                  <Input
                    id="rule-advertiser-name"
                    value={value.auction.advertiserName}
                    maxLength={128}
                    placeholder="例如 星河游戏"
                    required
                    onChange={(event) =>
                      change({
                        ...value,
                        auction: {
                          ...value.auction!,
                          advertiserName: event.target.value,
                        },
                      })
                    }
                  />
                </label>
                <p className="text-xs leading-5 text-muted-foreground sm:col-span-2">
                  同一广告主标识只提交一条代表计划；中标后按自己的出价计费。
                </p>
              </div>
            )}
            <div className="grid gap-3 sm:grid-cols-3">
              <label
                htmlFor="rule-daily-budget"
                className="space-y-1.5 text-sm"
              >
                日预算（元）
                <Input
                  id="rule-daily-budget"
                  aria-label="日预算（元）"
                  inputMode="decimal"
                  value={value.dailyBudgetYuan}
                  onChange={(event) =>
                    change({ ...value, dailyBudgetYuan: event.target.value })
                  }
                  required
                />
              </label>
              <label
                htmlFor="rule-impression-cost"
                className="space-y-1.5 text-sm"
              >
                {value.auction ? '单次曝光出价（元）' : '单次曝光成本（元）'}
                <Input
                  id="rule-impression-cost"
                  aria-label={
                    value.auction ? '单次曝光出价（元）' : '单次曝光成本（元）'
                  }
                  inputMode="decimal"
                  value={value.auction?.bidYuan ?? value.impressionCostYuan}
                  onChange={(event) =>
                    change(
                      value.auction
                        ? {
                            ...value,
                            auction: {
                              ...value.auction,
                              bidYuan: event.target.value,
                            },
                          }
                        : { ...value, impressionCostYuan: event.target.value },
                    )
                  }
                  required
                />
              </label>
              <label htmlFor="rule-frequency" className="space-y-1.5 text-sm">
                每人每日最多曝光次数
                <Input
                  id="rule-frequency"
                  aria-label="每人每日最多曝光次数"
                  type="number"
                  min={1}
                  max={100}
                  step={1}
                  value={value.frequencyLimit}
                  onChange={(event) =>
                    change({ ...value, frequencyLimit: event.target.value })
                  }
                  required
                />
              </label>
            </div>
            {ruleGroups.map((group) => (
              <section key={group} className="rounded-xl border p-3 sm:p-4">
                <div className="mb-3 flex items-start justify-between gap-2">
                  <div>
                    <h3 className="font-medium">{groups[group].title}</h3>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {groups[group].help}
                    </p>
                  </div>
                  {editable && (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        changeRows(group, [
                          ...value.targeting[group],
                          { tag: '' },
                        ])
                      }
                      aria-label={`${groups[group].title}：添加条件`}
                    >
                      <Plus />
                      添加
                    </Button>
                  )}
                </div>
                {value.targeting[group].length === 0 && (
                  <p className="text-sm text-muted-foreground">无条件</p>
                )}
                <div className="space-y-2">
                  {value.targeting[group].map((row, index) => (
                    <div
                      key={index}
                      className="flex flex-wrap gap-2 rounded-lg bg-muted/35 p-2"
                    >
                      <RuleConditionInput
                        row={row}
                        label={`${groups[group].title}第${index + 1}条`}
                        disabled={!editable || busy || preview !== null}
                        onChange={(next) => updateRow(group, index, next)}
                      />
                      {editable && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-label={`删除${groups[group].title}第${index + 1}条`}
                          onClick={() =>
                            changeRows(
                              group,
                              value.targeting[group].filter(
                                (_, i) => i !== index,
                              ),
                            )
                          }
                        >
                          <Trash2 className="text-destructive" />
                        </Button>
                      )}
                    </div>
                  ))}
                </div>
              </section>
            ))}
          </fieldset>
          {preview && (
            <section
              className="space-y-3 rounded-xl border border-primary/20 bg-primary/5 p-4"
              aria-label="发布前确认"
            >
              <h3 className="font-semibold">确认发布 v{version + 1}</h3>
              <p className="text-sm">
                {preview.auction
                  ? `一价竞价 · ${preview.auction.advertiserName}（${preview.auction.advertiserId}）`
                  : '固定成本 · 无可成交竞价计划时兜底'}
              </p>
              <p className="text-sm">
                日预算 ¥{(preview.dailyBudgetFen / 100).toFixed(2)}，单次成本 ¥
                {(preview.impressionCostFen / 100).toFixed(2)}，每日频控{' '}
                {preview.frequencyLimit}{' '}
                次。发布后按上述投放时间生效，不修改起止时间。
              </p>
              <TargetingSummary targeting={preview.targeting} />
            </section>
          )}
          {!editable && current.activeVersion && (
            <details className="rounded-lg border p-3">
              <summary className="cursor-pointer text-sm font-medium">
                查看当前版本 JSON
              </summary>
              <pre className="mt-3 max-h-60 overflow-auto text-xs">
                {JSON.stringify(current.activeVersion, null, 2)}
              </pre>
            </details>
          )}
          <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={onClose}
            >
              {editable ? '取消' : '关闭'}
            </Button>
            {editable && preview && (
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => setPreview(null)}
              >
                返回修改
              </Button>
            )}
            {editable && !preview && (
              <Button type="submit" disabled={busy}>
                预览并确认
              </Button>
            )}
            {editable && preview && (
              <Button
                type="button"
                disabled={busy}
                onClick={() => void publish()}
              >
                {busy && <LoaderCircle className="animate-spin" />}
                {busy ? '正在发布…' : `确认发布 v${version + 1}`}
              </Button>
            )}
          </div>
          {editable && (
            <p className="text-xs text-muted-foreground">
              未发布的修改仅保留在当前弹窗；关闭会放弃修改。发布后的历史版本不被覆盖。
            </p>
          )}
        </form>
      </DialogContent>
    </Dialog>
  );
}
