'use client';

import Image from 'next/image';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ImageIcon, LoaderCircle } from 'lucide-react';
import { api, Campaign, Creative } from '@/lib/api';
import {
  CreativeAssetPicker,
  CreativeDropZone,
} from '@/components/creative-asset-picker';
import { CreativeImage } from '@/components/creative-image';
import { FormSelect } from '@/components/form-select';
import { useAccess } from '@/components/auth-gate';
import { localCreativeImageURL, isDemoCreative } from '@/lib/demo-creatives';
import { DeleteResourceButton } from '@/components/delete-resource-button';
import {
  nextTestCreativeNumber,
  selectedCreativeCampaignID,
  testCreativeTitle,
} from '@/lib/creative-defaults';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { PageHeading } from '@/components/page-heading';
import { StatusBadge, Field, Empty } from './shared';

export function CreativesView({
  active,
  campaigns,
  busy,
  run,
  campaignID,
  onCampaignIDChange: setCampaignID,
  onTest,
}: {
  active: boolean;
  campaigns: Campaign[];
  busy: boolean;
  run: (action: () => Promise<void>, message: string) => Promise<void>;
  campaignID: string;
  onCampaignIDChange: (id: string) => void;
  onTest: (campaign: Campaign) => void;
}) {
  const { canOperate } = useAccess();
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
    if (!active) return;
    let live = true;
    void Promise.resolve().then(() => {
      if (live) void load(selectedCampaignID);
    });
    return () => {
      live = false;
      cancelCreativeLoad();
    };
  }, [active, selectedCampaignID, load, cancelCreativeLoad]);
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
        action={
          items.some((item) => item.status === 'ACTIVE') &&
          selectedCampaignID ? (
            <Button
              variant="outline"
              onClick={() => {
                const campaign = campaigns.find(
                  (item) => item.id === selectedCampaignID,
                );
                if (campaign) onTest(campaign);
              }}
            >
              测试此计划
            </Button>
          ) : undefined
        }
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
                      : '选择一份素材后添加'}
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
                            }, '素材已启用')
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
            <CardDescription>点击选用，也支持拖拽。</CardDescription>
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
