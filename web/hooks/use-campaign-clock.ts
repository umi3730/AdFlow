'use client';
import { useEffect, useState } from 'react';
import type { Campaign } from '@/lib/api';
import { nextCampaignClockDelay } from '@/lib/campaign-delivery';

export function useCampaignClock(campaigns: Campaign[]) {
  const [now, setNow] = useState<number | null>(null);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    function tick() {
      clearTimeout(timer);
      const time = Date.now();
      setNow(time);
      timer = setTimeout(tick, nextCampaignClockDelay(campaigns, time));
    }
    function visible() {
      if (!document.hidden) tick();
    }
    tick();
    window.addEventListener('focus', tick);
    document.addEventListener('visibilitychange', visible);
    return () => {
      clearTimeout(timer);
      window.removeEventListener('focus', tick);
      document.removeEventListener('visibilitychange', visible);
    };
  }, [campaigns]);
  return now;
}
