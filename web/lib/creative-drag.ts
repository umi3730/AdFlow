import { demoCreatives } from './demo-creatives.ts';

export const CREATIVE_DRAG_TYPE = 'application/x-adflow-creative-id';
export interface CreativeTransfer {
  types: readonly string[];
  getData: (type: string) => string;
  files?: { length: number };
}

export function acceptsCreativeDrag(transfer: CreativeTransfer) {
  return (
    transfer.types.includes(CREATIVE_DRAG_TYPE) &&
    !transfer.types.includes('Files') &&
    !transfer.files?.length
  );
}

export function droppedCreativeID(
  transfer: CreativeTransfer,
  disabled = false,
): string | null {
  if (disabled || !acceptsCreativeDrag(transfer)) return null;
  try {
    const id = transfer.getData(CREATIVE_DRAG_TYPE);
    return demoCreatives.some((asset) => asset.id === id) ? id : null;
  } catch {
    return null;
  }
}
