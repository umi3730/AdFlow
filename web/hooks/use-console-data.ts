'use client';

import { useCallback, useRef, useState } from 'react';
import { api, type Campaign, type Metrics } from '@/lib/api';

// One in-flight refresh plus one queued refresh covers overlapping mutations.
// The shell decides when data is needed; mounting does not trigger a duplicate load.
export function useConsoleData() {
  const refreshTask = useRef<Promise<void> | null>(null);
  const refreshQueued = useRef(false);
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [metrics, setMetrics] = useState<Record<string, Metrics>>({});
  const [connected, setConnected] = useState(false);
  const [hasCheckedConnection, setHasCheckedConnection] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
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
          const enriched = await Promise.all(
            result.items.map(async (campaign) => {
              try {
                const creatives = await api.listCreatives(campaign.id);
                return {
                  ...campaign,
                  activeCreativeCount: creatives.items.filter(
                    (item) => item.status === 'ACTIVE',
                  ).length,
                };
              } catch {
                return { ...campaign, activeCreativeCount: null };
              }
            }),
          );
          setCampaigns(enriched);
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

  return {
    campaigns,
    setCampaigns,
    metrics,
    connected,
    hasCheckedConnection,
    refreshing,
    lastUpdatedAt,
    refresh,
  };
}
