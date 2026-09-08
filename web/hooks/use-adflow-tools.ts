'use client';

import { useEffect } from 'react';
import { api } from '@/lib/api';
import { useAccess } from '@/components/auth-gate';

type ModelTool = {
  name: string;
  title: string;
  description: string;
  inputSchema: Record<string, unknown>;
  annotations: { readOnlyHint: boolean; untrustedContentHint: boolean };
  execute(input: unknown): Promise<unknown>;
};

declare global {
  interface Document {
    modelContext?: {
      registerTool(
        tool: ModelTool,
        options?: { signal?: AbortSignal },
      ): void | Promise<void>;
    };
  }
}

export function useAdFlowTools(
  onChanged: () => Promise<void>,
  navigate: (view: 'campaigns' | 'profiles') => void,
  notify: (message: string) => void,
) {
  const { canOperate } = useAccess();
  useEffect(() => {
    const context = document.modelContext;
    if (!context?.registerTool) return;
    const lifecycle = new AbortController();
    const register = (tool: ModelTool) => {
      void Promise.resolve(
        context.registerTool(tool, { signal: lifecycle.signal }),
      ).catch(() => {});
    };

    register({
      name: 'list_ad_campaigns',
      title: 'List AdFlow campaigns',
      description:
        'Read the current advertising campaigns and show the campaign workspace.',
      inputSchema: {
        type: 'object',
        properties: {},
        additionalProperties: false,
      },
      annotations: { readOnlyHint: true, untrustedContentHint: false },
      async execute() {
        const result = await api.listCampaigns();
        navigate('campaigns');
        await onChanged();
        return {
          count: result.items.length,
          campaigns: result.items.map(({ id, name, status, slotId }) => ({
            id,
            name,
            status,
            slotId,
          })),
        };
      },
    });

    if (canOperate)
      register({
        name: 'create_campaign_draft',
        title: 'Create campaign draft',
        description:
          'Create a seven-day draft campaign in AdFlow, then show it in the campaign workspace.',
        inputSchema: {
          type: 'object',
          properties: {
            name: { type: 'string', minLength: 2, maxLength: 128 },
            slotId: { type: 'string', minLength: 2, maxLength: 64 },
          },
          required: ['name', 'slotId'],
          additionalProperties: false,
        },
        annotations: { readOnlyHint: false, untrustedContentHint: false },
        async execute(input) {
          const value = input as { name?: string; slotId?: string };
          if (!value.name || !value.slotId)
            throw new Error('name and slotId are required');
          const start = new Date();
          const campaign = await api.createCampaign({
            name: value.name,
            slotId: value.slotId,
            startAt: start.toISOString(),
            endAt: new Date(start.getTime() + 7 * 86400000).toISOString(),
          });
          navigate('campaigns');
          notify('智能体已创建广告计划草稿');
          await onChanged();
          return {
            id: campaign.id,
            status: campaign.status,
            name: campaign.name,
            slotId: campaign.slotId,
          };
        },
      });

    return () => lifecycle.abort();
  }, [navigate, notify, onChanged, canOperate]);
}
